package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &KVSecretDataSource{}

// KVSecretDataSource reads a KV v1 secret from Tuck.
type KVSecretDataSource struct {
	client *tuckClient
}

func NewKVSecretDataSource() datasource.DataSource {
	return &KVSecretDataSource{}
}

func (d *KVSecretDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_kv_secret"
}

func (d *KVSecretDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Reads a KV v1 secret from Tuck.",
		Attributes: map[string]schema.Attribute{
			"path": schema.StringAttribute{
				Required:    true,
				Description: "Logical path of the secret.",
			},
			"value": schema.StringAttribute{
				Computed:    true,
				Sensitive:   true,
				Description: "Secret value at the path.",
			},
		},
	}
}

type kvSecretDataSourceModel struct {
	Path  types.String `tfsdk:"path"`
	Value types.String `tfsdk:"value"`
}

func (d *KVSecretDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *KVSecretDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config kvSecretDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	value, found, err := d.client.getSecret(ctx, config.Path.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading secret", err.Error())
		return
	}
	if !found {
		resp.Diagnostics.AddError("Secret not found", "path: "+config.Path.ValueString())
		return
	}
	config.Value = types.StringValue(value)
	resp.Diagnostics.Append(resp.State.Set(ctx, config)...)
}
