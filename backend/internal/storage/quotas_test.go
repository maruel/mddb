package storage

import "testing"

func TestMinEffective(t *testing.T) {
	tests := []struct {
		name string
		vals []int
		want int
	}{
		{"all inherit", []int{-1, -1, -1}, -1},
		{"single positive", []int{-1, 5, -1}, 5},
		{"zero beats positive", []int{10, -1, 0}, 0},
		{"min of two positives", []int{10, -1, 3}, 3},
		{"all positive", []int{10, 5, 20}, 5},
		{"first wins tie", []int{3, 3, 3}, 3},
		{"inherit skipped", []int{-1, 100, 200}, 100},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := minEffective(tt.vals...)
			if got != tt.want {
				t.Errorf("minEffective(%v) = %d, want %d", tt.vals, got, tt.want)
			}
		})
	}
}

func TestMinEffectiveInt64(t *testing.T) {
	tests := []struct {
		name string
		vals []int64
		want int64
	}{
		{"all inherit", []int64{-1, -1, -1}, -1},
		{"single positive", []int64{-1, 100, -1}, 100},
		{"zero beats positive", []int64{1000, -1, 0}, 0},
		{"min of two", []int64{1000, -1, 500}, 500},
		{"all positive", []int64{1000, 500, 2000}, 500},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := minEffectiveInt64(tt.vals...)
			if got != tt.want {
				t.Errorf("minEffectiveInt64(%v) = %d, want %d", tt.vals, got, tt.want)
			}
		})
	}
}

func TestEffectiveQuotas(t *testing.T) {
	tests := []struct {
		name   string
		server ResourceQuotas
		org    ResourceQuotas
		ws     ResourceQuotas
		want   ResourceQuotas
	}{
		{
			name:   "server only, org and ws inherit",
			server: ResourceQuotas{MaxPages: 1000, MaxStorageBytes: 1024, MaxRecordsPerTable: 10000},
			org:    ResourceQuotas{MaxPages: -1, MaxStorageBytes: -1, MaxRecordsPerTable: -1},
			ws:     ResourceQuotas{MaxPages: -1, MaxStorageBytes: -1, MaxRecordsPerTable: -1},
			want:   ResourceQuotas{MaxPages: 1000, MaxStorageBytes: 1024, MaxRecordsPerTable: 10000},
		},
		{
			name:   "ws overrides server with lower value",
			server: ResourceQuotas{MaxPages: 1000, MaxAssetSizeBytes: 50},
			org:    ResourceQuotas{MaxPages: -1, MaxAssetSizeBytes: -1},
			ws:     ResourceQuotas{MaxPages: 100, MaxAssetSizeBytes: 10},
			want:   ResourceQuotas{MaxPages: 100, MaxAssetSizeBytes: 10},
		},
		{
			name:   "org restricts further",
			server: ResourceQuotas{MaxPages: 1000, MaxTablesPerWorkspace: 100},
			org:    ResourceQuotas{MaxPages: 500, MaxTablesPerWorkspace: 50},
			ws:     ResourceQuotas{MaxPages: 800, MaxTablesPerWorkspace: -1},
			want:   ResourceQuotas{MaxPages: 500, MaxTablesPerWorkspace: 50},
		},
		{
			name:   "all layers set - min wins",
			server: ResourceQuotas{MaxColumnsPerTable: 50},
			org:    ResourceQuotas{MaxColumnsPerTable: 30},
			ws:     ResourceQuotas{MaxColumnsPerTable: 20},
			want:   ResourceQuotas{MaxColumnsPerTable: 20},
		},
		{
			name:   "ws inherits (-1) uses server",
			server: ResourceQuotas{MaxRecordsPerTable: 5000},
			org:    ResourceQuotas{MaxRecordsPerTable: -1},
			ws:     ResourceQuotas{MaxRecordsPerTable: -1},
			want:   ResourceQuotas{MaxRecordsPerTable: 5000},
		},
		{
			name:   "org disabled (0) propagates",
			server: ResourceQuotas{MaxPages: 1000},
			org:    ResourceQuotas{MaxPages: 0},
			ws:     ResourceQuotas{MaxPages: -1},
			want:   ResourceQuotas{MaxPages: 0},
		},
		{
			name:   "ws disabled (0) propagates",
			server: ResourceQuotas{MaxPages: 1000},
			org:    ResourceQuotas{MaxPages: -1},
			ws:     ResourceQuotas{MaxPages: 0},
			want:   ResourceQuotas{MaxPages: 0},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EffectiveQuotas(tt.server, tt.org, tt.ws)
			if got != tt.want {
				t.Errorf("EffectiveQuotas() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

// TestEffectiveParentLimits verifies that computing the parent ceiling for a workspace
// (i.e. the effective limit from server+org without any workspace contribution) works
// correctly when AllInheritResourceQuotas is used as the workspace placeholder.
// Previously, passing ResourceQuotas{} (all zeros) would cause every field to resolve
// to 0 ("disabled"), hiding the real server/org limits from the UI.
func TestEffectiveParentLimits(t *testing.T) {
	server := DefaultResourceQuotas() // all positive defaults
	tests := []struct {
		name string
		org  ResourceQuotas
		want ResourceQuotas
	}{
		{
			name: "org inherits – server values flow through",
			org:  AllInheritResourceQuotas(),
			want: server,
		},
		{
			name: "org restricts pages – lower value wins",
			org:  ResourceQuotas{MaxPages: 100, MaxStorageBytes: -1, MaxRecordsPerTable: -1, MaxAssetSizeBytes: -1, MaxTablesPerWorkspace: -1, MaxColumnsPerTable: -1},
			want: ResourceQuotas{MaxPages: 100, MaxStorageBytes: server.MaxStorageBytes, MaxRecordsPerTable: server.MaxRecordsPerTable, MaxAssetSizeBytes: server.MaxAssetSizeBytes, MaxTablesPerWorkspace: server.MaxTablesPerWorkspace, MaxColumnsPerTable: server.MaxColumnsPerTable},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EffectiveQuotas(server, tt.org, AllInheritResourceQuotas())
			if got != tt.want {
				t.Errorf("EffectiveQuotas(server, org, AllInherit) = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestResourceQuotas_Validate(t *testing.T) {
	t.Run("inherit (-1) valid", func(t *testing.T) {
		q := ResourceQuotas{
			MaxPages: -1, MaxStorageBytes: -1, MaxRecordsPerTable: -1,
			MaxAssetSizeBytes: -1, MaxTablesPerWorkspace: -1, MaxColumnsPerTable: -1,
		}
		if err := q.Validate(); err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})
	t.Run("disabled (0) valid", func(t *testing.T) {
		q := ResourceQuotas{}
		if err := q.Validate(); err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})
	t.Run("positive valid", func(t *testing.T) {
		q := DefaultResourceQuotas()
		if err := q.Validate(); err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})
	t.Run("below -1 pages invalid", func(t *testing.T) {
		q := ResourceQuotas{MaxPages: -2}
		if err := q.Validate(); err == nil {
			t.Error("expected error for MaxPages < -1")
		}
	})
	t.Run("below -1 storage invalid", func(t *testing.T) {
		q := ResourceQuotas{MaxStorageBytes: -2}
		if err := q.Validate(); err == nil {
			t.Error("expected error for MaxStorageBytes < -1")
		}
	})
}
