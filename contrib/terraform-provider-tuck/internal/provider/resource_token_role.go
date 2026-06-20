package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &TokenRoleResource{}

// TokenRoleResource manages a Tuck token role.
type TokenRoleResource struct {
	client *tuckClient
}

func NewTokenRoleResource() resource.Resource {
	return &TokenRoleResource{}
}

func (r *TokenRoleResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_token_role"
}

func (r *TokenRoleResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Tuck token role. The role name is immutable; changing it forces replacement.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Required:    true,
				Description: "Unique token role name.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"policies": schema.ListAttribute{
				Required:    true,
				ElementType: types.StringType,
				Description: "List of policy names attached to tokens created from this role.",
			},
			"ttl": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Default token lifetime (e.g. \"24h\"). Empty means no expiry.",
			},
			"max_ttl": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Maximum lifetime cap for renewals (e.g. \"168h\"). Empty means no cap.",
			},
			"max_uses": schema.Int64Attribute{
				Optional:    true,
				Computed:    true,
				Description: "Maximum number of uses per token. 0 means unlimited.",
			},
			"renewable": schema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(true),
				Description: "Whether tokens created from this role can be renewed.",
			},
			"period": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "If set, tokens are periodic and renewed to this duration on each renewal (e.g. \"4h\").",
			},
		},
	}
}

type tokenRoleModel struct {
	Name      types.String `tfsdk:"name"`
	Policies  types.List   `tfsdk:"policies"`
	TTL       types.String `tfsdk:"ttl"`
	MaxTTL    types.String `tfsdk:"max_ttl"`
	MaxUses   types.Int64  `tfsdk:"max_uses"`
	Renewable types.Bool   `tfsdk:"renewable"`
	Period    types.String `tfsdk:"period"`
}

func (r *TokenRoleResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *TokenRoleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan tokenRoleModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	apiReq := tokenRoleModelToReq(ctx, plan)
	if err := r.client.putTokenRole(ctx, plan.Name.ValueString(), apiReq); err != nil {
		resp.Diagnostics.AddError("Error creating token role", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *TokenRoleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state tokenRoleModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	role, found, err := r.client.getTokenRole(ctx, state.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading token role", err.Error())
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
	state.TTL = types.StringValue(nsDuration(role.TTL))
	state.MaxTTL = types.StringValue(nsDuration(role.MaxTTL))
	state.MaxUses = types.Int64Value(int64(role.MaxUses))
	state.Renewable = types.BoolValue(role.Renewable)
	state.Period = types.StringValue(nsDuration(role.Period))
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *TokenRoleResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan tokenRoleModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	apiReq := tokenRoleModelToReq(ctx, plan)
	if err := r.client.putTokenRole(ctx, plan.Name.ValueString(), apiReq); err != nil {
		resp.Diagnostics.AddError("Error updating token role", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *TokenRoleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state tokenRoleModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.deleteTokenRole(ctx, state.Name.ValueString()); err != nil {
		resp.Diagnostics.AddError("Error deleting token role", err.Error())
	}
}

func tokenRoleModelToReq(ctx context.Context, m tokenRoleModel) tokenRoleReq {
	var policies []string
	_ = m.Policies.ElementsAs(ctx, &policies, false)
	return tokenRoleReq{
		Policies:  policies,
		TTL:       m.TTL.ValueString(),
		MaxTTL:    m.MaxTTL.ValueString(),
		MaxUses:   int(m.MaxUses.ValueInt64()),
		Renewable: m.Renewable.ValueBool(),
		Period:    m.Period.ValueString(),
	}
}
