package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &SSHRoleResource{}

// SSHRoleResource manages a Tuck SSH CA role.
type SSHRoleResource struct {
	client *tuckClient
}

func NewSSHRoleResource() resource.Resource {
	return &SSHRoleResource{}
}

func (r *SSHRoleResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ssh_role"
}

func (r *SSHRoleResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Tuck SSH CA role, used to constrain which SSH certificates sign may issue against it. The role name is immutable; changing it forces replacement.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Required:    true,
				Description: "Unique SSH role name.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"allowed_users": schema.ListAttribute{
				Optional:    true,
				Computed:    true,
				ElementType: types.StringType,
				Description: "SSH usernames that may be requested as principals. Empty list allows any username.",
			},
			"default_extensions": schema.MapAttribute{
				Optional:    true,
				Computed:    true,
				ElementType: types.StringType,
				Description: "Extensions added to every certificate. Defaults to the standard permit-* set when unset.",
			},
			"cert_type": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "\"user\" (default) or \"host\".",
			},
			"default_ttl": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Default certificate lifetime (e.g. \"1h\") when a sign request doesn't specify one.",
			},
			"max_ttl": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Maximum certificate lifetime a sign request may ask for.",
			},
		},
	}
}

type sshRoleModel struct {
	Name              types.String `tfsdk:"name"`
	AllowedUsers      types.List   `tfsdk:"allowed_users"`
	DefaultExtensions types.Map    `tfsdk:"default_extensions"`
	CertType          types.String `tfsdk:"cert_type"`
	DefaultTTL        types.String `tfsdk:"default_ttl"`
	MaxTTL            types.String `tfsdk:"max_ttl"`
}

func (r *SSHRoleResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *SSHRoleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan sshRoleModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	apiReq := sshRoleModelToReq(ctx, plan)
	result, err := r.client.putSSHRole(ctx, plan.Name.ValueString(), apiReq)
	if err != nil {
		resp.Diagnostics.AddError("Error creating SSH role", err.Error())
		return
	}
	state, diags := sshRoleRespToModel(ctx, result)
	preservePlannedTTLs(&state.DefaultTTL, &state.MaxTTL, plan.DefaultTTL, plan.MaxTTL)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *SSHRoleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state sshRoleModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	role, found, err := r.client.getSSHRole(ctx, state.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading SSH role", err.Error())
		return
	}
	if !found {
		resp.State.RemoveResource(ctx)
		return
	}
	newState, diags := sshRoleRespToModel(ctx, role)
	newState.DefaultTTL = types.StringValue(preserveEquivalentTTL(newState.DefaultTTL.ValueString(), state.DefaultTTL))
	newState.MaxTTL = types.StringValue(preserveEquivalentTTL(newState.MaxTTL.ValueString(), state.MaxTTL))
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, newState)...)
}

func (r *SSHRoleResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan sshRoleModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	apiReq := sshRoleModelToReq(ctx, plan)
	result, err := r.client.putSSHRole(ctx, plan.Name.ValueString(), apiReq)
	if err != nil {
		resp.Diagnostics.AddError("Error updating SSH role", err.Error())
		return
	}
	state, diags := sshRoleRespToModel(ctx, result)
	preservePlannedTTLs(&state.DefaultTTL, &state.MaxTTL, plan.DefaultTTL, plan.MaxTTL)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *SSHRoleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state sshRoleModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.deleteSSHRole(ctx, state.Name.ValueString()); err != nil {
		resp.Diagnostics.AddError("Error deleting SSH role", err.Error())
	}
}

func sshRoleModelToReq(ctx context.Context, m sshRoleModel) sshRoleReq {
	var users []string
	_ = m.AllowedUsers.ElementsAs(ctx, &users, false)
	var exts map[string]string
	_ = m.DefaultExtensions.ElementsAs(ctx, &exts, false)
	return sshRoleReq{
		AllowedUsers:      users,
		DefaultExtensions: exts,
		CertType:          m.CertType.ValueString(),
		DefaultTTL:        m.DefaultTTL.ValueString(),
		MaxTTL:            m.MaxTTL.ValueString(),
	}
}

func sshRoleRespToModel(ctx context.Context, role *sshRoleAPIResp) (sshRoleModel, diag.Diagnostics) {
	var diags diag.Diagnostics
	users, d := types.ListValueFrom(ctx, types.StringType, role.AllowedUsers)
	diags.Append(d...)
	exts, d := types.MapValueFrom(ctx, types.StringType, role.DefaultExtensions)
	diags.Append(d...)
	return sshRoleModel{
		Name:              types.StringValue(role.Name),
		AllowedUsers:      users,
		DefaultExtensions: exts,
		CertType:          types.StringValue(role.CertType),
		DefaultTTL:        types.StringValue(nsDuration(role.DefaultTTL)),
		MaxTTL:            types.StringValue(nsDuration(role.MaxTTL)),
	}, diags
}
