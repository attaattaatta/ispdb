package app

import "testing"

func TestTrackedTablesIncludeUserLimitSources(t *testing.T) {
	t.Parallel()

	values := trackedTables()
	required := []string{"users", "userprops", "preset", "preset_props"}
	for _, item := range required {
		found := false
		for _, value := range values {
			if value == item {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("trackedTables() does not include %q: %#v", item, values)
		}
	}
}
