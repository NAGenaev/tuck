package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &KVSecretV2Resource{}

// KVSecretV2Resource manages a versioned KV v2 secret in Tuck.
type KVSecretV2Resource struct {
	client *tuckClient
}

func NewKVSecretV2Resource() resource.Resource {
	return &KVSecretV2Resource{}
}

func (r *KVSecretV2Resource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_kv_secret_v2"
}

func (r *KVSecretV2Resource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a KV v2 (versioned) secret in Tuck. Path is immutable; changing it forces replacement.",
		Attributes: map[string]schema.Attribute{
			"path": schema.StringAttribute{
				Required:    true,
				Description: "Logical path of the secret (e.g. \"app/config\").",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"value": schema.StringAttribute{
				Required:    true,
				Sensitive:   true,
				Description: "Raw secret value.",
			},
			"version": schema.Int64Attribute{
				Computed:    true,
				Description: "Version number assigned by Tuck after the last write.",
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

type kvSecretV2Model struct {
	Path    types.String `tfsdk:"path"`
	Value   types.String `tfsdk:"value"`
	Version types.Int64  `tfsdk:"version"`
}

func (r *KVSecretV2Resource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, ok := req.ProviderData.(*tuckClient)
	if !ok {
		resp.Diagnostics.AddError("unexpected provider data type",
			"Expected *tuckClient from provider.Configure.")
		return
	}
	r.client = client
}

func (r *KVSecretV2Resource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan kvSecretV2Model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	ver, err := r.client.putKVv2(ctx, plan.Path.ValueString(), plan.Value.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error creating KV v2 secret", err.Error())
		return
	}
	plan.Version = types.Int64Value(int64(ver))
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *KVSecretV2Resource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state kvSecretV2Model
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	result, found, err := r.client.getKVv2(ctx, state.Path.ValueString(), 0)
	if err != nil {
		resp.Diagnostics.AddError("Error reading KV v2 secret", err.Error())
		return
	}
	if !found || result.Deleted {
		resp.State.RemoveResource(ctx)
		return
	}
	state.Value = types.StringValue(result.Value)
	state.Version = types.Int64Value(int64(result.Version))
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *KVSecretV2Resource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan kvSecretV2Model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	ver, err := r.client.putKVv2(ctx, plan.Path.ValueString(), plan.Value.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error updating KV v2 secret", err.Error())
		return
	}
	plan.Version = types.Int64Value(int64(ver))
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *KVSecretV2Resource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state kvSecretV2Model
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.deleteKVv2(ctx, state.Path.ValueString()); err != nil {
		resp.Diagnostics.AddError("Error deleting KV v2 secret", err.Error())
	}
}
