package storage

import "testing"

func TestMinPositive(t *testing.T) {
	tests := []struct {
		name string
		vals []int
		want int
	}{
		{"all zero", []int{0, 0, 0}, 0},
		{"single positive", []int{0, 5, 0}, 5},
		{"min of two", []int{10, 0, 3}, 3},
		{"all positive", []int{10, 5, 20}, 5},
		{"first wins tie", []int{3, 3, 3}, 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := minPositive(tt.vals...)
			if got != tt.want {
				t.Errorf("minPositive(%v) = %d, want %d", tt.vals, got, tt.want)
			}
		})
	}
}

func TestMinPositiveInt64(t *testing.T) {
	tests := []struct {
		name string
		vals []int64
		want int64
	}{
		{"all zero", []int64{0, 0, 0}, 0},
		{"single positive", []int64{0, 100, 0}, 100},
		{"min of two", []int64{1000, 0, 500}, 500},
		{"all positive", []int64{1000, 500, 2000}, 500},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := minPositiveInt64(tt.vals...)
			if got != tt.want {
				t.Errorf("minPositiveInt64(%v) = %d, want %d", tt.vals, got, tt.want)
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
			name:   "server only",
			server: ResourceQuotas{MaxPages: 1000, MaxStorageBytes: 1024, MaxRecordsPerTable: 10000},
			org:    ResourceQuotas{},
			ws:     ResourceQuotas{},
			want:   ResourceQuotas{MaxPages: 1000, MaxStorageBytes: 1024, MaxRecordsPerTable: 10000},
		},
		{
			name:   "ws overrides server with lower value",
			server: ResourceQuotas{MaxPages: 1000, MaxAssetSizeBytes: 50},
			org:    ResourceQuotas{},
			ws:     ResourceQuotas{MaxPages: 100, MaxAssetSizeBytes: 10},
			want:   ResourceQuotas{MaxPages: 100, MaxAssetSizeBytes: 10},
		},
		{
			name:   "org restricts further",
			server: ResourceQuotas{MaxPages: 1000, MaxTablesPerWorkspace: 100},
			org:    ResourceQuotas{MaxPages: 500, MaxTablesPerWorkspace: 50},
			ws:     ResourceQuotas{MaxPages: 800},
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
			name:   "all zeros = unlimited",
			server: ResourceQuotas{},
			org:    ResourceQuotas{},
			ws:     ResourceQuotas{},
			want:   ResourceQuotas{},
		},
		{
			name:   "ws zero inherits from server",
			server: ResourceQuotas{MaxRecordsPerTable: 5000},
			org:    ResourceQuotas{},
			ws:     ResourceQuotas{MaxRecordsPerTable: 0},
			want:   ResourceQuotas{MaxRecordsPerTable: 5000},
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

func TestResourceQuotas_Validate(t *testing.T) {
	t.Run("valid zeros", func(t *testing.T) {
		q := ResourceQuotas{}
		if err := q.Validate(); err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})
	t.Run("valid positive", func(t *testing.T) {
		q := DefaultResourceQuotas()
		if err := q.Validate(); err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})
	t.Run("negative pages", func(t *testing.T) {
		q := ResourceQuotas{MaxPages: -1}
		if err := q.Validate(); err == nil {
			t.Error("expected error for negative MaxPages")
		}
	})
	t.Run("negative storage", func(t *testing.T) {
		q := ResourceQuotas{MaxStorageBytes: -1}
		if err := q.Validate(); err == nil {
			t.Error("expected error for negative MaxStorageBytes")
		}
	})
}
