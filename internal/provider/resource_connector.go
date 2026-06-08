package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/objectplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/microwave-sh/microwave-go/management"
)

var (
	_ resource.Resource                   = &ConnectorResource{}
	_ resource.ResourceWithConfigure      = &ConnectorResource{}
	_ resource.ResourceWithImportState    = &ConnectorResource{}
	_ resource.ResourceWithValidateConfig = &ConnectorResource{}
)

// ConnectorResource manages a workspace federation connector — the customer
// row a SYSTEM federation Trust Exchange (Terraform Cloud, GitHub Actions)
// resolves at policy time to bind an external workload identity to a
// Microwave workspace. Connectors are immutable after creation; every change
// to provider_type or the flavor block forces replacement.
type ConnectorResource struct {
	client *management.Client
}

type connectorModel struct {
	ID             types.String               `tfsdk:"id"`
	WorkspaceID    types.String               `tfsdk:"workspace_id"`
	ProviderType   types.String               `tfsdk:"provider_type"`
	TerraformCloud *terraformCloudClaimsModel `tfsdk:"terraform_cloud"`
	GitHubActions  *gitHubActionsClaimsModel  `tfsdk:"github_actions"`
	CreatedAt      types.String               `tfsdk:"created_at"`
	UpdatedAt      types.String               `tfsdk:"updated_at"`
}

type terraformCloudClaimsModel struct {
	Organization types.String `tfsdk:"organization"`
	Workspace    types.String `tfsdk:"workspace"`
}

type gitHubActionsClaimsModel struct {
	Repository types.String `tfsdk:"repository"`
	Workflow   types.String `tfsdk:"workflow"`
}

func NewConnectorResource() resource.Resource { return &ConnectorResource{} }

func (r *ConnectorResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_connector"
}

func (r *ConnectorResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, ok := req.ProviderData.(*management.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data", fmt.Sprintf("expected *management.Client, got %T", req.ProviderData))
		return
	}
	r.client = client
}

func (r *ConnectorResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "A Microwave workspace federation connector. Binds a Terraform Cloud organization+workspace pair (or a GitHub Actions repository+workflow pair) to this workspace so the matching SYSTEM federation Trust Exchange can resolve external OIDC tokens at policy-evaluation time. Immutable after creation — any change forces replacement.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				Description:   "Server-assigned connector ID.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"workspace_id": schema.StringAttribute{
				Required:    true,
				Description: "Workspace ID this connector belongs to. Immutable.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"provider_type": schema.StringAttribute{
				Required:    true,
				Description: "Federation provider shape. One of: terraform_cloud, github_actions. Must match the populated flavor block. Named provider_type (not provider) because Terraform reserves the bare 'provider' name as a resource meta-argument.",
				Validators: []validator.String{
					stringvalidator.OneOf(
						string(management.ConnectorProviderTerraformCloud),
						string(management.ConnectorProviderGitHubActions),
					),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"terraform_cloud": schema.SingleNestedAttribute{
				Optional:    true,
				Description: "Terraform Cloud claim binding. Set when provider_type = terraform_cloud.",
				Attributes: map[string]schema.Attribute{
					"organization": schema.StringAttribute{
						Required:    true,
						Description: "TFC organization name (matches the 'terraform_organization_name' claim).",
					},
					"workspace": schema.StringAttribute{
						Required:    true,
						Description: "TFC workspace name (matches the 'terraform_workspace_name' claim).",
					},
				},
				PlanModifiers: []planmodifier.Object{
					objectplanmodifier.RequiresReplace(),
				},
			},
			"github_actions": schema.SingleNestedAttribute{
				Optional:    true,
				Description: "GitHub Actions claim binding. Set when provider_type = github_actions.",
				Attributes: map[string]schema.Attribute{
					"repository": schema.StringAttribute{
						Required:    true,
						Description: "GitHub repository in 'owner/repo' form (matches the 'repository' claim).",
					},
					"workflow": schema.StringAttribute{
						Required:    true,
						Description: "Workflow file path (matches the 'workflow_ref' claim, e.g. .github/workflows/deploy.yml).",
					},
				},
				PlanModifiers: []planmodifier.Object{
					objectplanmodifier.RequiresReplace(),
				},
			},
			"created_at": schema.StringAttribute{
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"updated_at": schema.StringAttribute{
				Computed: true,
			},
		},
	}
}

// ValidateConfig enforces the "exactly-one flavor block, matching provider_type"
// invariant up front so a misconfigured connector fails at plan time with a
// useful diagnostic instead of at apply time with a 400 from the server.
func (r *ConnectorResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var cfg connectorModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	// Skip cross-field validation while provider_type is still unknown
	// (e.g. interpolated from another resource).
	if cfg.ProviderType.IsUnknown() || cfg.ProviderType.IsNull() {
		return
	}
	hasTFC := cfg.TerraformCloud != nil
	hasGHA := cfg.GitHubActions != nil
	switch {
	case hasTFC && hasGHA:
		resp.Diagnostics.AddError(
			"Multiple connector flavors set",
			"Exactly one of terraform_cloud or github_actions must be set; both were provided.",
		)
	case !hasTFC && !hasGHA:
		resp.Diagnostics.AddError(
			"No connector flavor set",
			"Exactly one of terraform_cloud or github_actions must be set; neither was provided.",
		)
	}
	switch cfg.ProviderType.ValueString() {
	case string(management.ConnectorProviderTerraformCloud):
		if !hasTFC {
			resp.Diagnostics.AddAttributeError(
				path.Root("terraform_cloud"),
				"Missing terraform_cloud block",
				"provider_type = terraform_cloud requires the terraform_cloud nested block.",
			)
		}
		if hasGHA {
			resp.Diagnostics.AddAttributeError(
				path.Root("github_actions"),
				"Wrong flavor block",
				"provider_type = terraform_cloud cannot carry a github_actions block.",
			)
		}
	case string(management.ConnectorProviderGitHubActions):
		if !hasGHA {
			resp.Diagnostics.AddAttributeError(
				path.Root("github_actions"),
				"Missing github_actions block",
				"provider_type = github_actions requires the github_actions nested block.",
			)
		}
		if hasTFC {
			resp.Diagnostics.AddAttributeError(
				path.Root("terraform_cloud"),
				"Wrong flavor block",
				"provider_type = github_actions cannot carry a terraform_cloud block.",
			)
		}
	}
}

func (r *ConnectorResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan connectorModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	out, err := r.client.Connectors.Create(ctx, plan.WorkspaceID.ValueString(), connectorToWire(&plan))
	if err != nil {
		resp.Diagnostics.AddError("Create connector failed", err.Error())
		return
	}
	connectorFromWire(&plan, out)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *ConnectorResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state connectorModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	out, err := r.client.Connectors.Get(ctx, state.WorkspaceID.ValueString(), state.ID.ValueString())
	if err != nil {
		if management.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Read connector failed", err.Error())
		return
	}
	connectorFromWire(&state, out)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update is a no-op: every meaningful change (workspace_id, provider_type,
// flavor block contents) is RequiresReplace, so the framework should never
// actually call Update. Implemented as a state-passthrough only to satisfy
// the resource.Resource interface contract.
func (r *ConnectorResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan connectorModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *ConnectorResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state connectorModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.Connectors.Delete(ctx, state.WorkspaceID.ValueString(), state.ID.ValueString()); err != nil && !management.IsNotFound(err) {
		resp.Diagnostics.AddError("Delete connector failed", err.Error())
	}
}

// ImportState accepts the composite "{workspace_id}/{connector_id}" form
// because connectors are workspace-scoped on the server and a bare ID alone
// can't address one.
func (r *ConnectorResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Expected: "<workspace_id>/<connector_id>"
	parts := splitOnce(req.ID, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		resp.Diagnostics.AddError(
			"Invalid import ID",
			"Expected '<workspace_id>/<connector_id>', got: "+req.ID,
		)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("workspace_id"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), parts[1])...)
}

func connectorToWire(m *connectorModel) *management.ConnectorInput {
	in := &management.ConnectorInput{
		Provider: management.ConnectorProvider(m.ProviderType.ValueString()),
	}
	if m.TerraformCloud != nil {
		in.TerraformCloud = &management.TerraformCloudClaims{
			Organization: m.TerraformCloud.Organization.ValueString(),
			Workspace:    m.TerraformCloud.Workspace.ValueString(),
		}
	}
	if m.GitHubActions != nil {
		in.GitHubActions = &management.GitHubActionsClaims{
			Repository: m.GitHubActions.Repository.ValueString(),
			Workflow:   m.GitHubActions.Workflow.ValueString(),
		}
	}
	return in
}

func connectorFromWire(m *connectorModel, out *management.Connector) {
	m.ID = types.StringValue(out.ID)
	m.WorkspaceID = types.StringValue(out.WorkspaceID)
	m.ProviderType = types.StringValue(string(out.Provider))
	if out.TerraformCloud != nil {
		m.TerraformCloud = &terraformCloudClaimsModel{
			Organization: types.StringValue(out.TerraformCloud.Organization),
			Workspace:    types.StringValue(out.TerraformCloud.Workspace),
		}
	} else {
		m.TerraformCloud = nil
	}
	if out.GitHubActions != nil {
		m.GitHubActions = &gitHubActionsClaimsModel{
			Repository: types.StringValue(out.GitHubActions.Repository),
			Workflow:   types.StringValue(out.GitHubActions.Workflow),
		}
	} else {
		m.GitHubActions = nil
	}
	m.CreatedAt = types.StringValue(out.CreatedAt.Format(timeFormat))
	m.UpdatedAt = types.StringValue(out.UpdatedAt.Format(timeFormat))
}

// splitOnce splits s on the first occurrence of sep. Returns a single-element
// slice when sep is absent so callers can length-check.
func splitOnce(s, sep string) []string {
	for i := 0; i+len(sep) <= len(s); i++ {
		if s[i:i+len(sep)] == sep {
			return []string{s[:i], s[i+len(sep):]}
		}
	}
	return []string{s}
}
