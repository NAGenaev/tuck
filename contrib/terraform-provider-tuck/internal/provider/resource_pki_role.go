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

var _ resource.Resource = &PKIRoleResource{}

// PKIRoleResource manages a Tuck PKI role.
type PKIRoleResource struct {
	client *tuckClient
}

func NewPKIRoleResource() resource.Resource {
	return &PKIRoleResource{}
}

func (r *PKIRoleResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pki_role"
}

func (r *PKIRoleResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Tuck PKI role, used to constrain which certificates issue_cert may issue against it. The role name is immutable; changing it forces replacement.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Required:    true,
				Description: "Unique PKI role name.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"allowed_domains": schema.ListAttribute{
				Optional:    true,
				Computed:    true,
				ElementType: types.StringType,
				Description: "Domains certificates issued under this role may use as a common name.",
			},
			"allow_subdomains": schema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Whether subdomains of allowed_domains may also be issued.",
			},
			"allow_ip_sans": schema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Whether IP address Subject Alternative Names are permitted.",
			},
			"allow_localhost": schema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Whether \"localhost\" is always permitted as a common name.",
			},
			"key_type": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Key algorithm for issued certificates: \"ec\" (default) or \"rsa\".",
			},
			"key_bits": schema.Int64Attribute{
				Optional:    true,
				Computed:    true,
				Description: "Key size in bits. Only meaningful for key_type = \"rsa\".",
			},
			"default_ttl": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Default certificate lifetime (e.g. \"720h\") when a request doesn't specify one.",
			},
			"max_ttl": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Maximum certificate lifetime a request may ask for.",
			},
			"server_flag": schema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Whether issued certificates get the TLS server-auth extended key usage.",
			},
			"client_flag": schema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Whether issued certificates get the TLS client-auth extended key usage.",
			},
		},
	}
}

type pkiRoleModel struct {
	Name            types.String `tfsdk:"name"`
	AllowedDomains  types.List   `tfsdk:"allowed_domains"`
	AllowSubdomains types.Bool   `tfsdk:"allow_subdomains"`
	AllowIPSANs     types.Bool   `tfsdk:"allow_ip_sans"`
	AllowLocalhost  types.Bool   `tfsdk:"allow_localhost"`
	KeyType         types.String `tfsdk:"key_type"`
	KeyBits         types.Int64  `tfsdk:"key_bits"`
	DefaultTTL      types.String `tfsdk:"default_ttl"`
	MaxTTL          types.String `tfsdk:"max_ttl"`
	ServerFlag      types.Bool   `tfsdk:"server_flag"`
	ClientFlag      types.Bool   `tfsdk:"client_flag"`
}

func (r *PKIRoleResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *PKIRoleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan pkiRoleModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	apiReq := pkiRoleModelToReq(ctx, plan)
	result, err := r.client.putPKIRole(ctx, plan.Name.ValueString(), apiReq)
	if err != nil {
		resp.Diagnostics.AddError("Error creating PKI role", err.Error())
		return
	}
	state, diags := pkiRoleRespToModel(ctx, result)
	preservePlannedTTLs(&state.DefaultTTL, &state.MaxTTL, plan.DefaultTTL, plan.MaxTTL)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *PKIRoleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state pkiRoleModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	role, found, err := r.client.getPKIRole(ctx, state.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading PKI role", err.Error())
		return
	}
	if !found {
		resp.State.RemoveResource(ctx)
		return
	}
	newState, diags := pkiRoleRespToModel(ctx, role)
	newState.DefaultTTL = types.StringValue(preserveEquivalentTTL(newState.DefaultTTL.ValueString(), state.DefaultTTL))
	newState.MaxTTL = types.StringValue(preserveEquivalentTTL(newState.MaxTTL.ValueString(), state.MaxTTL))
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, newState)...)
}

func (r *PKIRoleResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan pkiRoleModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	apiReq := pkiRoleModelToReq(ctx, plan)
	result, err := r.client.putPKIRole(ctx, plan.Name.ValueString(), apiReq)
	if err != nil {
		resp.Diagnostics.AddError("Error updating PKI role", err.Error())
		return
	}
	state, diags := pkiRoleRespToModel(ctx, result)
	preservePlannedTTLs(&state.DefaultTTL, &state.MaxTTL, plan.DefaultTTL, plan.MaxTTL)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *PKIRoleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state pkiRoleModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.deletePKIRole(ctx, state.Name.ValueString()); err != nil {
		resp.Diagnostics.AddError("Error deleting PKI role", err.Error())
	}
}

func pkiRoleModelToReq(ctx context.Context, m pkiRoleModel) pkiRoleReq {
	var domains []string
	_ = m.AllowedDomains.ElementsAs(ctx, &domains, false)
	return pkiRoleReq{
		AllowedDomains:  domains,
		AllowSubdomains: m.AllowSubdomains.ValueBool(),
		AllowIPSANs:     m.AllowIPSANs.ValueBool(),
		AllowLocalhost:  m.AllowLocalhost.ValueBool(),
		KeyType:         m.KeyType.ValueString(),
		KeyBits:         int(m.KeyBits.ValueInt64()),
		DefaultTTL:      m.DefaultTTL.ValueString(),
		MaxTTL:          m.MaxTTL.ValueString(),
		ServerFlag:      m.ServerFlag.ValueBool(),
		ClientFlag:      m.ClientFlag.ValueBool(),
	}
}

func pkiRoleRespToModel(ctx context.Context, role *pkiRoleAPIResp) (pkiRoleModel, diag.Diagnostics) {
	domains, diags := types.ListValueFrom(ctx, types.StringType, role.AllowedDomains)
	return pkiRoleModel{
		Name:            types.StringValue(role.Name),
		AllowedDomains:  domains,
		AllowSubdomains: types.BoolValue(role.AllowSubdomains),
		AllowIPSANs:     types.BoolValue(role.AllowIPSANs),
		AllowLocalhost:  types.BoolValue(role.AllowLocalhost),
		KeyType:         types.StringValue(role.KeyType),
		KeyBits:         types.Int64Value(int64(role.KeyBits)),
		DefaultTTL:      types.StringValue(nsDuration(role.DefaultTTL)),
		MaxTTL:          types.StringValue(nsDuration(role.MaxTTL)),
		ServerFlag:      types.BoolValue(role.ServerFlag),
		ClientFlag:      types.BoolValue(role.ClientFlag),
	}, diags
}
