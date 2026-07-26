package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &TransitKeyResource{}

// TransitKeyResource manages a Tuck Transit encryption key. Transit has no
// "update key" operation for name/type — key rotation adds a new version but
// isn't a Terraform-managed attribute — so both are RequiresReplace.
type TransitKeyResource struct {
	client *tuckClient
}

func NewTransitKeyResource() resource.Resource {
	return &TransitKeyResource{}
}

func (r *TransitKeyResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_transit_key"
}

func (r *TransitKeyResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Tuck Transit encryption key (encrypt/decrypt/sign/verify as a service). name and type are immutable; changing either forces replacement.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Required:    true,
				Description: "Unique Transit key name.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"type": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Key algorithm, e.g. \"aes256-gcm96\" (default), \"ecdsa-p256\", \"ed25519\".",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"deletion_allowed": schema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
				Description: "Tuck refuses to delete a Transit key by default. Set this to true (and apply) before removing the resource, or `terraform destroy` fails with a clear error instead of silently succeeding without actually deleting key material.",
			},
			"latest_version": schema.Int64Attribute{
				Computed:    true,
				Description: "Current key version number.",
			},
		},
	}
}

type transitKeyModel struct {
	Name            types.String `tfsdk:"name"`
	Type            types.String `tfsdk:"type"`
	DeletionAllowed types.Bool   `tfsdk:"deletion_allowed"`
	LatestVersion   types.Int64  `tfsdk:"latest_version"`
}

func (r *TransitKeyResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *TransitKeyResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan transitKeyModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	result, err := r.client.createTransitKey(ctx, plan.Name.ValueString(), plan.Type.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error creating Transit key", err.Error())
		return
	}
	if plan.DeletionAllowed.ValueBool() {
		if err := r.client.setTransitKeyDeletable(ctx, plan.Name.ValueString(), true); err != nil {
			resp.Diagnostics.AddError("Error setting Transit key deletion_allowed", err.Error())
			return
		}
		result.Deletable = true
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, transitKeyRespToModel(result))...)
}

func (r *TransitKeyResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state transitKeyModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	key, found, err := r.client.getTransitKey(ctx, state.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading Transit key", err.Error())
		return
	}
	if !found {
		resp.State.RemoveResource(ctx)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, transitKeyRespToModel(key))...)
}

// Update only ever has deletion_allowed to apply — name and type are
// RequiresReplace, so a change to either goes through Delete+Create instead.
func (r *TransitKeyResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan transitKeyModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.setTransitKeyDeletable(ctx, plan.Name.ValueString(), plan.DeletionAllowed.ValueBool()); err != nil {
		resp.Diagnostics.AddError("Error updating Transit key deletion_allowed", err.Error())
		return
	}
	key, found, err := r.client.getTransitKey(ctx, plan.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading Transit key after update", err.Error())
		return
	}
	if !found {
		resp.State.RemoveResource(ctx)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, transitKeyRespToModel(key))...)
}

func (r *TransitKeyResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state transitKeyModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !state.DeletionAllowed.ValueBool() {
		resp.Diagnostics.AddError(
			"Transit key deletion not allowed",
			fmt.Sprintf("deletion_allowed is false for key %q. Set deletion_allowed = true and apply before destroying this resource.", state.Name.ValueString()),
		)
		return
	}
	if err := r.client.deleteTransitKey(ctx, state.Name.ValueString()); err != nil {
		resp.Diagnostics.AddError("Error deleting Transit key", err.Error())
	}
}

func transitKeyRespToModel(key *transitKeyAPIResp) transitKeyModel {
	return transitKeyModel{
		Name:            types.StringValue(key.Name),
		Type:            types.StringValue(key.Type),
		DeletionAllowed: types.BoolValue(key.Deletable),
		LatestVersion:   types.Int64Value(int64(key.LatestVersion)),
	}
}
