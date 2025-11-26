package provider

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"strings"

	"github.com/ceph/go-ceph/rgw/admin"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

const accessKeyBytes = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ"

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.ResourceWithConfigure = &UserResource{}
var _ resource.ResourceWithImportState = &UserResource{}
var _ resource.ResourceWithImportState = &UserResource{}

func NewUserResource() resource.Resource {
	return &UserResource{}
}

type UserResource struct {
	client *RgwClient
}

type UserResourceModel struct {
	Id                     types.String   `tfsdk:"id"`
	Username               types.String   `tfsdk:"username"`
	DisplayName            types.String   `tfsdk:"display_name"`
	Email                  types.String   `tfsdk:"email"`
	GenerateS3Credentials  types.Bool     `tfsdk:"generate_s3_credentials"`
	ExclusiveS3Credentials types.Bool     `tfsdk:"exclusive_s3_credentials"`
	Caps                   []UserCapModel `tfsdk:"caps"`
	OpMask                 types.String   `tfsdk:"op_mask"`
	MaxBuckets             types.Int64    `tfsdk:"max_buckets"`
	Suspended              types.Bool     `tfsdk:"suspended"`
	Tenant                 types.String   `tfsdk:"tenant"`
	AccessKey              types.String   `tfsdk:"access_key"`
	SecretKey              types.String   `tfsdk:"secret_key"`
	PurgeDataOnDelete      types.Bool     `tfsdk:"purge_data_on_delete"`
	Principal              types.String   `tfsdk:"principal"`
}

type UserCapModel struct {
	Type types.String `tfsdk:"type"`
	Perm types.String `tfsdk:"perm"`
}

func (r *UserResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_user"
}

func (r *UserResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Ceph RGW User",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
					stringplanmodifier.RequiresReplace(),
				},
			},
			"username": schema.StringAttribute{
				MarkdownDescription: "The user ID to be created (without tenant).",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.NoneOf("$"),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"display_name": schema.StringAttribute{
				MarkdownDescription: "Display Name of user",
				Required:            true,
			},
			"email": schema.StringAttribute{
				MarkdownDescription: "The email address associated with the user.",
				Optional:            true,
			},
			"generate_s3_credentials": schema.BoolAttribute{
				Description:         "Specify whether to generate S3 Credentials for the user",
				MarkdownDescription: "Specify whether to generate S3 Credentials for the user. Set to false to generate swift keys via rgw_subuser.",
				Optional:            true,
			},
			"exclusive_s3_credentials": schema.BoolAttribute{
				Description:         "Specify whether other s3 credentials for this user not managed by this ressource should be deleted.",
				MarkdownDescription: "Specify how to deal with s3 credentials for this user not managed by this resource. Set to `true` to delete all other s3 credentials. Set to `false` to ignore other credentials.",
				Optional:            true,
			},
			"caps": schema.ListNestedAttribute{
				Optional: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"type": schema.StringAttribute{
							Required: true,
						},
						"perm": schema.StringAttribute{
							Required: true,
						},
					},
				},
			},
			"op_mask": schema.StringAttribute{
				MarkdownDescription: "The op-mask of the user",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringDefaultModifier{"read, write, delete"},
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"max_buckets": schema.Int64Attribute{
				MarkdownDescription: "Specify the maximum number of buckets the user can own.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.Int64{
					int64DefaultModifier{1000},
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"suspended": schema.BoolAttribute{
				MarkdownDescription: "Specify whether the user should be suspended.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.Bool{
					boolDefaultModifier{false},
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"tenant": schema.StringAttribute{
				MarkdownDescription: "The tenant under which a user is a part of.",
				Optional:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"access_key": schema.StringAttribute{
				MarkdownDescription: "The generated access key",
				Computed:            true,
			},
			"secret_key": schema.StringAttribute{
				MarkdownDescription: "The generated secret key",
				Computed:            true,
				Sensitive:           true,
			},
			"purge_data_on_delete": schema.BoolAttribute{
				MarkdownDescription: "Purge user data on deletion",
				Optional:            true,
			},
			"principal": schema.StringAttribute{
				MarkdownDescription: "Computed principal to be used in policies",
				Computed:            true,
			},
		},
	}
}

func (r *UserResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	// Prevent panic if the provider has not been configured.
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*RgwClient)

	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *RgwClient, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)

		return
	}

	r.client = client
}

func (r *UserResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	// Read Terraform plan data into the model
	var data *UserResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Create API user object
	rgwUser := admin.User{
		DisplayName: data.DisplayName.ValueString(),
		Email:       data.Email.ValueString(),
		OpMask:      data.OpMask.ValueString(),
	}
	if data.Tenant.IsNull() {
		rgwUser.ID = data.Username.ValueString()
	} else {
		rgwUser.ID = fmt.Sprintf("%s$%s", data.Tenant.ValueString(), data.Username.ValueString())
	}
	generateKey := false
	if data.GenerateS3Credentials.ValueBool() || data.GenerateS3Credentials.IsNull() {
		generateKey = true
		rgwUser.KeyType = "s3"
	}
	rgwUser.GenerateKey = &generateKey

	if len(data.Caps) > 0 {
		rgwUser.Caps = make([]admin.UserCapSpec, len(data.Caps))
		for i, c := range data.Caps {
			rgwUser.Caps[i] = admin.UserCapSpec{
				Type: c.Type.ValueString(),
				Perm: c.Type.ValueString(),
			}
		}
	}

	maxBuckets := int(data.MaxBuckets.ValueInt64())
	rgwUser.MaxBuckets = &maxBuckets

	suspended := 0
	if data.Suspended.ValueBool() {
		suspended = 1
	}
	rgwUser.Suspended = &suspended

	// create user
	createdUser, err := r.client.Admin.CreateUser(ctx, rgwUser)
	if err != nil {
		resp.Diagnostics.AddError("could not create user", err.Error())
		return
	}

	// set resource id - use the constructed ID to ensure consistency
	data.Id = types.StringValue(rgwUser.ID)

	// set principal ARN
	if data.Tenant.IsNull() {
		data.Principal = types.StringValue(fmt.Sprintf("arn:aws:iam:::user/%s", data.Username.ValueString()))
	} else {
		data.Principal = types.StringValue(fmt.Sprintf("arn:aws:iam::%s:user/%s", data.Tenant.ValueString(), data.Username.ValueString()))
	}

	// set access and secret key
	if generateKey {
		if len(createdUser.Keys) == 1 {
			data.AccessKey = types.StringValue(createdUser.Keys[0].AccessKey)
			data.SecretKey = types.StringValue(createdUser.Keys[0].SecretKey)
		} else {
			resp.Diagnostics.AddAttributeError(path.Root("access_key"), "api didn't return exactly one s3 key pair", fmt.Sprintf("expected one s3 api key pair in api response, got %d", len(createdUser.Keys)))
			resp.Diagnostics.AddAttributeError(path.Root("secret_key"), "api didn't return exactly one s3 key pair", fmt.Sprintf("expected one s3 api key pair in api response, got %d", len(createdUser.Keys)))
		}
	} else {
		data.AccessKey = types.StringNull()
		data.SecretKey = types.StringNull()
	}

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *UserResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	// Read Terraform prior state data into the model
	var data *UserResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// prepare request attributes
	reqUser := admin.User{
		ID: data.Id.ValueString(),
	}

	// get user
	user, err := r.client.Admin.GetUser(ctx, reqUser)
	if err != nil {
		if errors.Is(err, admin.ErrNoSuchUser) {
			// Remove user from state
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("could not get user", err.Error())
		return
	}

	// check resource id - handle both full ID (tenant$username) and username-only formats
	expectedId := data.Id.ValueString()
	actualId := user.ID

	// If we expect tenant$username but got username, reconstruct the expected username
	if strings.Contains(expectedId, "$") {
		parts := strings.SplitN(expectedId, "$", 2)
		if len(parts) == 2 && actualId != expectedId && actualId == parts[1] {
			// API returned username only, but we expected tenant$username - this is acceptable
			// We'll update username/tenant fields below based on the stored ID
		} else if actualId != expectedId {
			resp.Diagnostics.AddError("api returned wrong user", fmt.Sprintf("expected user '%s', got '%s'", expectedId, actualId))
			return
		}
	} else if actualId != expectedId {
		resp.Diagnostics.AddError("api returned wrong user", fmt.Sprintf("expected user '%s', got '%s'", expectedId, actualId))
		return
	}

	// update username and tenant based on the stored ID (not the API response)
	// Use the expected ID from state to handle cases where API returns different format
	idToSplit := expectedId
	splittedId := strings.SplitN(idToSplit, "$", 2)
	if len(splittedId) == 2 {
		data.Username = types.StringValue(splittedId[1])
		data.Tenant = types.StringValue(splittedId[0])
	} else {
		data.Username = types.StringValue(idToSplit)
		data.Tenant = types.StringNull()
	}

	// update display name
	data.DisplayName = types.StringValue(user.DisplayName)

	// update email
	if len(user.Email) > 0 || !data.Email.IsNull() {
		data.Email = types.StringValue(user.Email)
	}
	if user.Email == "" && len(data.Email.ValueString()) > 0 {
		data.Email = types.StringValue("")
	}

	// update caps
	if len(user.Caps) > 0 {
		data.Caps = make([]UserCapModel, len(user.Caps))
		for i, c := range user.Caps {
			data.Caps[i].Type = types.StringValue(c.Type)
			data.Caps[i].Perm = types.StringValue(c.Perm)
		}
	} else {
		user.Caps = nil
	}

	// update max_buckets
	if user.MaxBuckets != nil {
		data.MaxBuckets = types.Int64Value(int64(*user.MaxBuckets))
	}

	// update suspended
	if user.Suspended != nil {
		if *user.Suspended < 1 {
			data.Suspended = types.BoolValue(false)
		} else {
			data.Suspended = types.BoolValue(true)
		}
	}

	// update credentials
	if data.GenerateS3Credentials.ValueBool() || data.GenerateS3Credentials.IsNull() {
		found := false
		if data.AccessKey.IsNull() || data.AccessKey.IsUnknown() {
			resp.Diagnostics.Append(resp.Private.SetKey(ctx, "mark_unknown_access_key", []byte("1"))...)
		} else {
			for _, k := range user.Keys {
				if k.AccessKey == data.AccessKey.ValueString() {
					found = true
					data.SecretKey = types.StringValue(k.SecretKey)
					resp.Diagnostics.Append(resp.Private.SetKey(ctx, "mark_unknown_access_key", []byte("0"))...)
					resp.Diagnostics.Append(resp.Private.SetKey(ctx, "mark_unknown_secret_key", []byte("0"))...)
					break
				}
			}
		}
		if !found {
			resp.Diagnostics.Append(resp.Private.SetKey(ctx, "mark_unknown_secret_key", []byte("1"))...)
		}
		if len(user.Keys) > 1 || (len(user.Keys) == 1 && !found) {
			data.ExclusiveS3Credentials = types.BoolValue(false)
		}
	} else {
		resp.Diagnostics.Append(resp.Private.SetKey(ctx, "mark_unknown_access_key", []byte("0"))...)
		resp.Diagnostics.Append(resp.Private.SetKey(ctx, "mark_unknown_secret_key", []byte("0"))...)
		data.AccessKey = types.StringNull()
		data.SecretKey = types.StringNull()
	}

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *UserResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	// Read Terraform plan data into the model
	var data *UserResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// instantiate api request user struct
	update := admin.User{
		ID:          data.Id.ValueString(),
		DisplayName: data.DisplayName.ValueString(),
		Email:       data.Email.ValueString(),
		OpMask:      data.OpMask.ValueString(),
	}

	// do not generate key here
	generate := false
	update.GenerateKey = &generate

	// set user caps
	if len(data.Caps) > 0 {
		update.Caps = make([]admin.UserCapSpec, len(data.Caps))
		for i, c := range data.Caps {
			update.Caps[i] = admin.UserCapSpec{
				Type: c.Type.ValueString(),
				Perm: c.Type.ValueString(),
			}
		}
	}

	// set max_buckets
	maxBuckets := int(data.MaxBuckets.ValueInt64())
	update.MaxBuckets = &maxBuckets

	// set suspended
	suspended := 0
	if data.Suspended.ValueBool() {
		suspended = 1
	}
	update.Suspended = &suspended

	// modify user
	user, err := r.client.Admin.ModifyUser(ctx, update)
	if err != nil {
		resp.Diagnostics.AddError("could not modify user", err.Error())
		return
	}

	// Preserve existing S3 credentials during updates - only regenerate if explicitly requested
	// Read existing state to get current credentials
	var state *UserResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// If we have existing credentials in state, preserve them
	if !state.AccessKey.IsNull() && !state.SecretKey.IsNull() {
		data.AccessKey = state.AccessKey
		data.SecretKey = state.SecretKey
		data.Principal = state.Principal // Preserve the principal ARN as well
	} else if len(user.Keys) > 0 {
		// If no state credentials but API has keys, use the first one
		data.AccessKey = types.StringValue(user.Keys[0].AccessKey)
		data.SecretKey = types.StringValue(user.Keys[0].SecretKey)
		// Set principal ARN
		if data.Tenant.IsNull() {
			data.Principal = types.StringValue(fmt.Sprintf("arn:aws:iam:::user/%s", data.Username.ValueString()))
		} else {
			data.Principal = types.StringValue(fmt.Sprintf("arn:aws:iam::%s:user/%s", data.Tenant.ValueString(), data.Username.ValueString()))
		}
	} else {
		// No existing credentials and no API keys - this shouldn't happen in normal updates
		// but if it does, generate new credentials
		if data.GenerateS3Credentials.ValueBool() || data.GenerateS3Credentials.IsNull() {
			// Generate new access key
			a := make([]byte, 20)
			for i := range a {
				a[i] = accessKeyBytes[rand.Intn(len(accessKeyBytes))]
			}
			data.AccessKey = types.StringValue(string(a))

			generate := true
			keys, err := r.client.Admin.CreateKey(ctx, admin.UserKeySpec{
				UID:         user.ID,
				KeyType:     "s3",
				GenerateKey: &generate,
				AccessKey:   data.AccessKey.ValueString(),
			})
			if err != nil {
				resp.Diagnostics.AddError("could not generate s3 credentials", err.Error())
				return
			}

			if keys != nil && len(*keys) > 0 {
				for _, k := range *keys {
					if k.AccessKey == data.AccessKey.ValueString() {
						data.SecretKey = types.StringValue(k.SecretKey)
						break
					}
				}
			}

			// Set principal ARN
			if data.Tenant.IsNull() {
				data.Principal = types.StringValue(fmt.Sprintf("arn:aws:iam:::user/%s", data.Username.ValueString()))
			} else {
				data.Principal = types.StringValue(fmt.Sprintf("arn:aws:iam::%s:user/%s", data.Tenant.ValueString(), data.Username.ValueString()))
			}
		}
	}

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *UserResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	// Read Terraform prior state data into the model
	var data *UserResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// send delete request to api
	purgeData := 0
	if data.PurgeDataOnDelete.ValueBool() {
		purgeData = 1
	}
	err := r.client.Admin.RemoveUser(ctx, admin.User{
		ID:        data.Id.ValueString(),
		PurgeData: &purgeData,
	})
	if err != nil && !errors.Is(err, admin.ErrNoSuchUser) {
		resp.Diagnostics.AddError("could not delete user", err.Error())
		return
	}
}

/*
	type boolEnforceDefaultValueModifier struct {
		Default bool
	}

	func (m boolEnforceDefaultValueModifier) Description(ctx context.Context) string {
		return fmt.Sprintf("If value is not configured, enforces %t", m.Default)
	}

	func (m boolEnforceDefaultValueModifier) MarkdownDescription(ctx context.Context) string {
		return fmt.Sprintf("If value is not configured, enforces `%t`", m.Default)
	}

	func (m boolEnforceDefaultValueModifier) PlanModifyBool(ctx context.Context, req planmodifier.BoolRequest, resp *planmodifier.BoolResponse) {
		if req.ConfigValue.IsNull() {
			resp.PlanValue = types.BoolValue(m.Default)
			tflog.Info(ctx, "Enforcing default value")
		}
	}
*/
type stringPrivateUnknownModifier struct {
	Suffix string
}

func (m stringPrivateUnknownModifier) Description(ctx context.Context) string {
	return fmt.Sprintf("Set field to unknown if private provider data key 'mark_unknown_%s' contains '1'", m.Suffix)
}

func (m stringPrivateUnknownModifier) MarkdownDescription(ctx context.Context) string {
	return fmt.Sprintf("Set field to unknown if private provider data key 'mark_unknown_%s' contains '1'", m.Suffix)
}

func (m stringPrivateUnknownModifier) PlanModifyString(ctx context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) {
	data, diag := req.Private.GetKey(ctx, fmt.Sprintf("mark_unknown_%s", m.Suffix))
	resp.Diagnostics.Append(diag...)

	if data != nil {
		if string(data) == "1" {
			resp.PlanValue = types.StringUnknown()
		}
	}
}

func (r *UserResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// The import ID should be the full user ID (tenant$username or just username)
	userId := req.ID

	// Use the ID as-is for the resource ID
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)

	// Also set the ID in the response state for immediate use
	resp.State.SetAttribute(ctx, path.Root("id"), userId)

	// Fetch user details to import existing S3 credentials
	user, err := r.client.Admin.GetUser(ctx, admin.User{ID: userId})
	if err != nil {
		resp.Diagnostics.AddError("could not get user for import", err.Error())
		return
	}

	// Import user attributes
	resp.State.SetAttribute(ctx, path.Root("op_mask"), user.OpMask)

	// Set principal ARN
	splittedId := strings.SplitN(userId, "$", 2)
	if len(splittedId) == 2 {
		resp.State.SetAttribute(ctx, path.Root("principal"), fmt.Sprintf("arn:aws:iam::%s:user/%s", splittedId[0], splittedId[1]))
	} else {
		resp.State.SetAttribute(ctx, path.Root("principal"), fmt.Sprintf("arn:aws:iam:::user/%s", userId))
	}

	// Import existing S3 credentials if they exist
	if len(user.Keys) > 0 {
		resp.State.SetAttribute(ctx, path.Root("access_key"), user.Keys[0].AccessKey)
		resp.State.SetAttribute(ctx, path.Root("secret_key"), user.Keys[0].SecretKey)

		// Set exclusive credentials based on number of keys
		if len(user.Keys) > 1 {
			resp.State.SetAttribute(ctx, path.Root("exclusive_s3_credentials"), false)
		}
	}
}
