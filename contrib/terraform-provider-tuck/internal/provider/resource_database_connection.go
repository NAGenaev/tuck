package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &DatabaseConnectionResource{}

// DatabaseConnectionResource manages a Tuck dynamic-secrets database
// connection (the target Postgres/MySQL instance dynamic roles generate
// credentials against).
type DatabaseConnectionResource struct {
	client *tuckClient
}

func NewDatabaseConnectionResource() resource.Resource {
	return &DatabaseConnectionResource{}
}

func (r *DatabaseConnectionResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_database_connection"
}

func (r *DatabaseConnectionResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Tuck dynamic-secrets database connection. The connection name is immutable; changing it forces replacement.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Required:    true,
				Description: "Tuck's identifier for this connection config (not necessarily a real database name).",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"plugin_name": schema.StringAttribute{
				Required:    true,
				Description: "\"postgresql\" or \"mysql\".",
			},
			"connection_url": schema.StringAttribute{
				Required:    true,
				Sensitive:   true,
				Description: "DSN for the target database. May contain {{username}} and {{password}} placeholders for the root credential used to create/revoke dynamic users. Tuck never returns this value back on read (the API redacts it), so Terraform always trusts the configured value rather than server state for this attribute.",
			},
			"database": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Real database name substituted for {{database}} in role creation/revocation statements. Falls back to name server-side if unset — set explicitly whenever name isn't a valid SQL identifier (e.g. contains '-').",
			},
			"max_open_conns": schema.Int64Attribute{
				Optional:    true,
				Computed:    true,
				Description: "Maximum open connections Tuck keeps to this database. Defaults to 5.",
			},
		},
	}
}

type databaseConnectionModel struct {
	Name          types.String `tfsdk:"name"`
	PluginName    types.String `tfsdk:"plugin_name"`
	ConnectionURL types.String `tfsdk:"connection_url"`
	Database      types.String `tfsdk:"database"`
	MaxOpenConns  types.Int64  `tfsdk:"max_open_conns"`
}

func (r *DatabaseConnectionResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *DatabaseConnectionResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan databaseConnectionModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	result, err := r.client.putDBConnection(ctx, plan.Name.ValueString(), databaseConnectionModelToReq(plan))
	if err != nil {
		resp.Diagnostics.AddError("Error creating database connection", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, databaseConnectionRespToModel(result, plan.ConnectionURL))...)
}

func (r *DatabaseConnectionResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state databaseConnectionModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	cfg, found, err := r.client.getDBConnection(ctx, state.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading database connection", err.Error())
		return
	}
	if !found {
		resp.State.RemoveResource(ctx)
		return
	}
	// connection_url comes back "[redacted]" from the API — always keep
	// whatever Terraform already has, never overwrite it from a read.
	resp.Diagnostics.Append(resp.State.Set(ctx, databaseConnectionRespToModel(cfg, state.ConnectionURL))...)
}

func (r *DatabaseConnectionResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan databaseConnectionModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	result, err := r.client.putDBConnection(ctx, plan.Name.ValueString(), databaseConnectionModelToReq(plan))
	if err != nil {
		resp.Diagnostics.AddError("Error updating database connection", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, databaseConnectionRespToModel(result, plan.ConnectionURL))...)
}

func (r *DatabaseConnectionResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state databaseConnectionModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.deleteDBConnection(ctx, state.Name.ValueString()); err != nil {
		resp.Diagnostics.AddError("Error deleting database connection", err.Error())
	}
}

func databaseConnectionModelToReq(m databaseConnectionModel) dbConnectionReq {
	return dbConnectionReq{
		PluginName:    m.PluginName.ValueString(),
		ConnectionURL: m.ConnectionURL.ValueString(),
		Database:      m.Database.ValueString(),
		MaxOpenConns:  int(m.MaxOpenConns.ValueInt64()),
	}
}

// databaseConnectionRespToModel builds the model from an API response,
// substituting connectionURL for the (redacted) value the API returned —
// callers pass either the just-applied plan value or the pre-existing state
// value, never the raw API response's connection_url field.
func databaseConnectionRespToModel(cfg *dbConnectionAPIResp, connectionURL types.String) databaseConnectionModel {
	return databaseConnectionModel{
		Name:          types.StringValue(cfg.Name),
		PluginName:    types.StringValue(cfg.PluginName),
		ConnectionURL: connectionURL,
		Database:      types.StringValue(cfg.Database),
		MaxOpenConns:  types.Int64Value(int64(cfg.MaxOpenConns)),
	}
}
