package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &DatabaseRoleResource{}

// DatabaseRoleResource manages a Tuck dynamic-secrets database role.
type DatabaseRoleResource struct {
	client *tuckClient
}

func NewDatabaseRoleResource() resource.Resource {
	return &DatabaseRoleResource{}
}

func (r *DatabaseRoleResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_database_role"
}

func (r *DatabaseRoleResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Tuck dynamic-secrets database role — generate_creds against this role returns a short-lived database user. The role name is immutable; changing it forces replacement.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Required:    true,
				Description: "Unique database role name.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"db_name": schema.StringAttribute{
				Required:    true,
				Description: "Name of the tuck_database_connection this role generates credentials against.",
			},
			"creation_statements": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "SQL executed to create the dynamic user. Template vars: {{username}}, {{password}}, {{expiry}}, {{database}}. Defaults to a safe per-engine template when unset.",
			},
			"revocation_statements": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "SQL executed on lease expiry/revocation. Same template vars as creation_statements. Defaults to a safe per-engine template when unset.",
			},
			"default_ttl": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Default credential lifetime (e.g. \"1h\") when generate_creds doesn't specify one.",
			},
			"max_ttl": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Maximum credential lifetime.",
			},
		},
	}
}

type databaseRoleModel struct {
	Name                 types.String `tfsdk:"name"`
	DBName               types.String `tfsdk:"db_name"`
	CreationStatements   types.String `tfsdk:"creation_statements"`
	RevocationStatements types.String `tfsdk:"revocation_statements"`
	DefaultTTL           types.String `tfsdk:"default_ttl"`
	MaxTTL               types.String `tfsdk:"max_ttl"`
}

func (r *DatabaseRoleResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *DatabaseRoleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan databaseRoleModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	result, err := r.client.putDBRole(ctx, plan.Name.ValueString(), databaseRoleModelToReq(plan))
	if err != nil {
		resp.Diagnostics.AddError("Error creating database role", err.Error())
		return
	}
	state := databaseRoleRespToModel(result)
	preservePlannedTTLs(&state.DefaultTTL, &state.MaxTTL, plan.DefaultTTL, plan.MaxTTL)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *DatabaseRoleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state databaseRoleModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	role, found, err := r.client.getDBRole(ctx, state.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading database role", err.Error())
		return
	}
	if !found {
		resp.State.RemoveResource(ctx)
		return
	}
	newState := databaseRoleRespToModel(role)
	newState.DefaultTTL = types.StringValue(preserveEquivalentTTL(newState.DefaultTTL.ValueString(), state.DefaultTTL))
	newState.MaxTTL = types.StringValue(preserveEquivalentTTL(newState.MaxTTL.ValueString(), state.MaxTTL))
	resp.Diagnostics.Append(resp.State.Set(ctx, newState)...)
}

func (r *DatabaseRoleResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan databaseRoleModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	result, err := r.client.putDBRole(ctx, plan.Name.ValueString(), databaseRoleModelToReq(plan))
	if err != nil {
		resp.Diagnostics.AddError("Error updating database role", err.Error())
		return
	}
	state := databaseRoleRespToModel(result)
	preservePlannedTTLs(&state.DefaultTTL, &state.MaxTTL, plan.DefaultTTL, plan.MaxTTL)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *DatabaseRoleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state databaseRoleModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.deleteDBRole(ctx, state.Name.ValueString()); err != nil {
		resp.Diagnostics.AddError("Error deleting database role", err.Error())
	}
}

func databaseRoleModelToReq(m databaseRoleModel) dbRoleReq {
	return dbRoleReq{
		DBName:               m.DBName.ValueString(),
		CreationStatements:   m.CreationStatements.ValueString(),
		RevocationStatements: m.RevocationStatements.ValueString(),
		DefaultTTL:           m.DefaultTTL.ValueString(),
		MaxTTL:               m.MaxTTL.ValueString(),
	}
}

func databaseRoleRespToModel(role *dbRoleAPIResp) databaseRoleModel {
	return databaseRoleModel{
		Name:                 types.StringValue(role.Name),
		DBName:               types.StringValue(role.DBName),
		CreationStatements:   types.StringValue(role.CreationStatements),
		RevocationStatements: types.StringValue(role.RevocationStatements),
		DefaultTTL:           types.StringValue(nsDuration(role.DefaultTTL)),
		MaxTTL:               types.StringValue(nsDuration(role.MaxTTL)),
	}
}
