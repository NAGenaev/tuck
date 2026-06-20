package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &AppRoleRoleResource{}

// AppRoleRoleResource manages a Tuck AppRole role.
type AppRoleRoleResource struct {
	client *tuckClient
}

func NewAppRoleRoleResource() resource.Resource {
	return &AppRoleRoleResource{}
}

func (r *AppRoleRoleResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_approle_role"
}

func (r *AppRoleRoleResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Tuck AppRole role. The role name is immutable; changing it forces replacement.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Required:    true,
				Description: "Unique AppRole role name.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"policies": schema.ListAttribute{
				Required:    true,
				ElementType: types.StringType,
				Description: "Policies attached to tokens issued via this role.",
			},
			"token_ttl": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Lifetime of tokens issued via this role (e.g. \"1h\").",
			},
			"secret_id_ttl": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "How long generated SecretIDs remain valid (e.g. \"24h\"). Empty means no expiry.",
			},
			"secret_id_num_uses": schema.Int64Attribute{
				Optional:    true,
				Computed:    true,
				Description: "Maximum uses per SecretID. 0 means unlimited.",
			},
			"role_id": schema.StringAttribute{
				Computed:    true,
				Sensitive:   false,
				Description: "The auto-generated or server-assigned RoleID for this AppRole.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

type appRoleRoleModel struct {
	Name            types.String `tfsdk:"name"`
	Policies        types.List   `tfsdk:"policies"`
	TokenTTL        types.String `tfsdk:"token_ttl"`
	SecretIDTTL     types.String `tfsdk:"secret_id_ttl"`
	SecretIDNumUses types.Int64  `tfsdk:"secret_id_num_uses"`
	RoleID          types.String `tfsdk:"role_id"`
}

func (r *AppRoleRoleResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *AppRoleRoleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan appRoleRoleModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	apiReq := appRoleModelToReq(ctx, plan)
	result, err := r.client.putAppRole(ctx, plan.Name.ValueString(), apiReq)
	if err != nil {
		resp.Diagnostics.AddError("Error creating AppRole role", err.Error())
		return
	}
	plan.RoleID = types.StringValue(result.RoleID)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *AppRoleRoleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state appRoleRoleModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	role, found, err := r.client.getAppRole(ctx, state.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading AppRole role", err.Error())
		return
	}
	if !found {
		resp.State.RemoveResource(ctx)
		return
	}
	policies, diags := types.ListValueFrom(ctx, types.StringType, role.Policies)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	state.Policies = policies
	state.TokenTTL = types.StringValue(nsDuration(role.TokenTTL))
	state.SecretIDTTL = types.StringValue(nsDuration(role.SecretIDTTL))
	state.SecretIDNumUses = types.Int64Value(int64(role.SecretIDNumUses))
	state.RoleID = types.StringValue(role.RoleID)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *AppRoleRoleResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan appRoleRoleModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	apiReq := appRoleModelToReq(ctx, plan)
	result, err := r.client.putAppRole(ctx, plan.Name.ValueString(), apiReq)
	if err != nil {
		resp.Diagnostics.AddError("Error updating AppRole role", err.Error())
		return
	}
	plan.RoleID = types.StringValue(result.RoleID)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *AppRoleRoleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state appRoleRoleModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.deleteAppRole(ctx, state.Name.ValueString()); err != nil {
		resp.Diagnostics.AddError("Error deleting AppRole role", err.Error())
	}
}

func appRoleModelToReq(ctx context.Context, m appRoleRoleModel) appRoleReq {
	var policies []string
	_ = m.Policies.ElementsAs(ctx, &policies, false)
	return appRoleReq{
		Policies:        policies,
		TokenTTL:        m.TokenTTL.ValueString(),
		SecretIDTTL:     m.SecretIDTTL.ValueString(),
		SecretIDNumUses: int(m.SecretIDNumUses.ValueInt64()),
	}
}
