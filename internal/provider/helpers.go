package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// diagnostics is a local alias so resource files don't need to import the diag
// package directly — it appears in nearly every helper signature and the long
// fully-qualified name clutters the call sites.
type diagnostics = diag.Diagnostics

// stringListToSlice unwraps a types.List of strings to a Go slice. Returns
// an empty slice (not nil) when the list is null/unknown so wire-format
// JSON marshalling emits [] rather than null for required-but-empty fields.
func stringListToSlice(ctx context.Context, list types.List) ([]string, diagnostics) {
	if list.IsNull() || list.IsUnknown() {
		return []string{}, nil
	}
	out := make([]string, 0, len(list.Elements()))
	diags := list.ElementsAs(ctx, &out, false)
	return out, diags
}

// stringSliceToList is the inverse — used to write a wire-format slice back
// into TF state. The result is a typed list so consumers of the state can
// reliably destructure it.
func stringSliceToList(ctx context.Context, vals []string) (types.List, diagnostics) {
	if vals == nil {
		vals = []string{}
	}
	return types.ListValueFrom(ctx, types.StringType, vals)
}

// stringMapToAny converts a map of TF string values to a map of any, used
// when building wire-format request bodies for map fields (identity, output_claims).
func stringMapToAny(in map[string]types.String) map[string]any {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]any, len(in))
	for key, value := range in {
		out[key] = value.ValueString()
	}
	return out
}

// anyMapToStringMap converts a map[string]any wire response back into a
// map of TF string values. Non-string values are stringified via fmt.Sprint.
func anyMapToStringMap(in map[string]any) map[string]types.String {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]types.String, len(in))
	for key, value := range in {
		if s, ok := value.(string); ok {
			out[key] = types.StringValue(s)
			continue
		}
		out[key] = types.StringValue(fmt.Sprint(value))
	}
	return out
}
