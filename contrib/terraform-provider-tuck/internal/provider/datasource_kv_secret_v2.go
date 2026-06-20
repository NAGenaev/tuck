package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &KVSecretV2DataSource{}

// KVSecretV2DataSource reads a KV v2 (versioned) secret from Tuck.
type KVSecretV2DataSource struct {
	client *tuckClient
}

func NewKVSecretV2DataSource() datasource.DataSource {
	return &KVSecretV2DataSource{}
}

func (d *KVSecretV2DataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_kv_secret_v2"
}

func (d *KVSecretV2DataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Reads a KV v2 versioned secret from Tuck.",
		Attributes: map[string]schema.Attribute{
			"path": schema.StringAttribute{
				Required:    true,
				Description: "Logical path of the secret.",
			},
			"version": schema.Int64Attribute{
				Optional:    true,
				Computed:    true,
				Description: "Version to read. Omit or set 0 for the latest version.",
			},
			"value": schema.StringAttribute{
				Computed:    true,
				Sensitive:   true,
				Description: "Secret value at the requested version.",
			},
		},
	}
}

type kvSecretV2DSModel struct {
	Path    types.String `tfsdk:"path"`
	Version types.Int64  `tfsdk:"version"`
	Value   types.String `tfsdk:"value"`
}

func (d *KVSecretV2DataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, ok := req.ProviderData.(*tuckClient)
	if !ok {
		resp.Diagnostics.AddError("unexpected provider data type",
			"Expected *tuckClient from provider.Configure.")
		return
	}
	d.client = client
}

func (d *KVSecretV2DataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config kvSecretV2DSModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	ver := int(config.Version.ValueInt64())
	result, found, err := d.client.getKVv2(ctx, config.Path.ValueString(), ver)
	if err != nil {
		resp.Diagnostics.AddError("Error reading KV v2 secret", err.Error())
		return
	}
	if !found || result.Deleted {
		resp.Diagnostics.AddError("KV v2 secret not found", "path: "+config.Path.ValueString())
		return
	}
	config.Value = types.StringValue(result.Value)
	config.Version = types.Int64Value(int64(result.Version))
	resp.Diagnostics.Append(resp.State.Set(ctx, config)...)
}
