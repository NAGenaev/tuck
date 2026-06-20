package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &NamespaceResource{}

// NamespaceResource manages a Tuck namespace.
type NamespaceResource struct {
	client *tuckClient
}

func NewNamespaceResource() resource.Resource {
	return &NamespaceResource{}
}

func (r *NamespaceResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_namespace"
}

func (r *NamespaceResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Tuck namespace for multi-tenancy. The namespace name is immutable.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Required:    true,
				Description: "Unique namespace name (e.g. \"team-a\").",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
		},
	}
}

type namespaceModel struct {
	Name types.String `tfsdk:"name"`
}

func (r *NamespaceResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *NamespaceResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan namespaceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.createNamespace(ctx, plan.Name.ValueString()); err != nil {
		resp.Diagnostics.AddError("Error creating namespace", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *NamespaceResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state namespaceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	exists, err := r.client.namespaceExists(ctx, state.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading namespace", err.Error())
		return
	}
	if !exists {
		resp.State.RemoveResource(ctx)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *NamespaceResource) Update(_ context.Context, _ resource.UpdateRequest, _ *resource.UpdateResponse) {
	// Namespace has no mutable fields; name is ForceNew, so Update is never called.
}

func (r *NamespaceResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state namespaceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.deleteNamespace(ctx, state.Name.ValueString()); err != nil {
		resp.Diagnostics.AddError("Error deleting namespace", err.Error())
	}
}
