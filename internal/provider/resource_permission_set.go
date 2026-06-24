package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/microwave-sh/microwave-go/management"
)

var (
	_ resource.Resource                = &PermissionSetResource{}
	_ resource.ResourceWithConfigure   = &PermissionSetResource{}
	_ resource.ResourceWithImportState = &PermissionSetResource{}
)

// PermissionSetResource is the IaC handle for an RBAC permission bundle.
// Permissions is a wholesale-replacement list — the underlying API doesn't
// support partial diffs, so Update sends the full desired state.
type PermissionSetResource struct {
	client *management.Client
}

// permissionSetModel mirrors the resource block. Permissions uses a typed
// slice rather than types.List so the nested element shape stays type-safe
// across Create/Read/Update.
type permissionSetModel struct {
	ID          types.String      `tfsdk:"id"`
	Name        types.String      `tfsdk:"name"`
	Description types.String      `tfsdk:"description"`
	Permissions []permissionModel `tfsdk:"permissions"`
	CreatedAt   types.String      `tfsdk:"created_at"`
	UpdatedAt   types.String      `tfsdk:"updated_at"`
}

type permissionModel struct {
	Name        types.String `tfsdk:"name"`
	Label       types.String `tfsdk:"label"`
	Description types.String `tfsdk:"description"`
	Dangerous   types.Bool   `tfsdk:"dangerous"`
}

func NewPermissionSetResource() resource.Resource { return &PermissionSetResource{} }

func (r *PermissionSetResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_permission_set"
}

func (r *PermissionSetResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *PermissionSetResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "A Microwave permission set: a named bundle of scope grants bound into one or more key specs.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				Description:   "Microwave-assigned permission set ID (ps_...).",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "Permission set name. Unique within the workspace.",
			},
			"description": schema.StringAttribute{
				Optional:    true,
				Description: "Human-readable description.",
			},
			"permissions": schema.ListNestedAttribute{
				Required:    true,
				Description: "Scope grants in this set. Sent wholesale on every Update — the API does not support partial diffs.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							Required:    true,
							Description: "Scope string the API enforces (e.g. \"deploys:write\", or \"*\" for full access).",
						},
						"label": schema.StringAttribute{
							Required:    true,
							Description: "Human-readable title for the scope.",
						},
						"description": schema.StringAttribute{
							Optional:    true,
							Description: "Optional longer description of the grant.",
						},
						"dangerous": schema.BoolAttribute{
							Optional:    true,
							Computed:    true,
							Default:     booldefault.StaticBool(false),
							Description: "Marks grants that warrant extra confirmation in UIs.",
						},
					},
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

func (r *PermissionSetResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan permissionSetModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	out, err := r.client.PermissionSets.Create(ctx, &management.PermissionSetInput{
		Name:        plan.Name.ValueString(),
		Description: plan.Description.ValueString(),
		Permissions: permissionsToWire(plan.Permissions),
	})
	if err != nil {
		resp.Diagnostics.AddError("Create permission set failed", err.Error())
		return
	}
	plan.ID = types.StringValue(out.ID)
	plan.CreatedAt = types.StringValue(out.CreatedAt.Format(timeFormat))
	plan.UpdatedAt = types.StringValue(out.UpdatedAt.Format(timeFormat))
	plan.Permissions = permissionsFromWire(out.Permissions)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *PermissionSetResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state permissionSetModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	out, err := r.client.PermissionSets.Get(ctx, state.ID.ValueString())
	if err != nil {
		if management.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Read permission set failed", err.Error())
		return
	}
	state.Name = types.StringValue(out.Name)
	state.Description = types.StringValue(out.Description)
	state.Permissions = permissionsFromWire(out.Permissions)
	state.CreatedAt = types.StringValue(out.CreatedAt.Format(timeFormat))
	state.UpdatedAt = types.StringValue(out.UpdatedAt.Format(timeFormat))
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *PermissionSetResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan permissionSetModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	out, err := r.client.PermissionSets.Update(ctx, plan.ID.ValueString(), &management.PermissionSetInput{
		Name:        plan.Name.ValueString(),
		Description: plan.Description.ValueString(),
		Permissions: permissionsToWire(plan.Permissions),
	})
	if err != nil {
		resp.Diagnostics.AddError("Update permission set failed", err.Error())
		return
	}
	plan.Permissions = permissionsFromWire(out.Permissions)
	plan.UpdatedAt = types.StringValue(out.UpdatedAt.Format(timeFormat))
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *PermissionSetResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state permissionSetModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.PermissionSets.Delete(ctx, state.ID.ValueString()); err != nil && !management.IsNotFound(err) {
		resp.Diagnostics.AddError("Delete permission set failed", err.Error())
	}
}

func (r *PermissionSetResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func permissionsToWire(in []permissionModel) []management.PermissionInput {
	out := make([]management.PermissionInput, 0, len(in))
	for _, p := range in {
		out = append(out, management.PermissionInput{
			Name:        p.Name.ValueString(),
			Label:       p.Label.ValueString(),
			Description: p.Description.ValueString(),
			Dangerous:   p.Dangerous.ValueBool(),
		})
	}
	return out
}

func permissionsFromWire(in []management.Permission) []permissionModel {
	out := make([]permissionModel, 0, len(in))
	for _, p := range in {
		m := permissionModel{
			Name:      types.StringValue(p.Name),
			Label:     types.StringValue(p.Label),
			Dangerous: types.BoolValue(p.Dangerous),
		}
		if p.Description != "" {
			m.Description = types.StringValue(p.Description)
		} else {
			m.Description = types.StringNull()
		}
		out = append(out, m)
	}
	return out
}
