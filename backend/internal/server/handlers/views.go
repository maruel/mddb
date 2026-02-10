// Handles view operations.

package handlers

import (
	"context"

	"github.com/maruel/mddb/backend/internal/ksid"
	"github.com/maruel/mddb/backend/internal/server/dto"
	"github.com/maruel/mddb/backend/internal/storage"
	"github.com/maruel/mddb/backend/internal/storage/content"
	"github.com/maruel/mddb/backend/internal/storage/identity"
)

// ViewHandler handles view requests.
type ViewHandler struct {
	Svc *Services
	Cfg *Config
}

// CreateView creates a new view for a table.
func (h *ViewHandler) CreateView(ctx context.Context, wsID ksid.ID, user *identity.User, req *dto.CreateViewRequest) (*dto.CreateViewResponse, error) {
	ws, err := h.Svc.FileStore.GetWorkspaceStore(ctx, wsID)
	if err != nil {
		return nil, dto.InternalWithError("Failed to get workspace", err)
	}

	node, err := ws.ReadTable(req.NodeID)
	if err != nil {
		return nil, dto.NotFound("table")
	}

	viewID := ksid.NewID()
	view := content.View{
		ID:   viewID,
		Name: req.Name,
		Type: content.ViewType(req.Type),
	}

	node.Views = append(node.Views, view)
	node.Modified = storage.Now()

	author := GitAuthor(user)
	if err := ws.WriteTable(ctx, node, false, author); err != nil {
		return nil, dto.InternalWithError("Failed to save view", err)
	}

	return &dto.CreateViewResponse{ID: viewID}, nil
}

// UpdateView updates an existing view.
func (h *ViewHandler) UpdateView(ctx context.Context, wsID ksid.ID, user *identity.User, req *dto.UpdateViewRequest) (*dto.UpdateViewResponse, error) {
	ws, err := h.Svc.FileStore.GetWorkspaceStore(ctx, wsID)
	if err != nil {
		return nil, dto.InternalWithError("Failed to get workspace", err)
	}

	node, err := ws.ReadTable(req.NodeID)
	if err != nil {
		return nil, dto.NotFound("table")
	}

	found := false
	for i := range node.Views {
		if node.Views[i].ID != req.ViewID {
			continue
		}
		// Update fields if present
		if req.Name != "" {
			node.Views[i].Name = req.Name
		}
		if req.Type != "" {
			node.Views[i].Type = content.ViewType(req.Type)
		}
		// For slices, we assume the client sends the full new state if they are non-nil
		if req.Columns != nil {
			node.Views[i].Columns = viewColumnsToEntity(req.Columns)
		}
		if req.Filters != nil {
			node.Views[i].Filters = filtersToEntity(req.Filters)
		}
		if req.Sorts != nil {
			node.Views[i].Sorts = sortsToEntity(req.Sorts)
		}
		if req.Groups != nil {
			node.Views[i].Groups = groupsToEntity(req.Groups)
		}
		found = true
		break
	}

	if !found {
		return nil, dto.NotFound("view")
	}

	node.Modified = storage.Now()
	author := GitAuthor(user)
	if err := ws.WriteTable(ctx, node, false, author); err != nil {
		return nil, dto.InternalWithError("Failed to update view", err)
	}

	return &dto.UpdateViewResponse{ID: req.ViewID}, nil
}

// DeleteView deletes a view.
func (h *ViewHandler) DeleteView(ctx context.Context, wsID ksid.ID, user *identity.User, req *dto.DeleteViewRequest) (*dto.DeleteViewResponse, error) {
	ws, err := h.Svc.FileStore.GetWorkspaceStore(ctx, wsID)
	if err != nil {
		return nil, dto.InternalWithError("Failed to get workspace", err)
	}

	node, err := ws.ReadTable(req.NodeID)
	if err != nil {
		return nil, dto.NotFound("table")
	}

	newViews := make([]content.View, 0, len(node.Views))
	found := false
	for i := range node.Views {
		if node.Views[i].ID == req.ViewID {
			found = true
			continue
		}
		newViews = append(newViews, node.Views[i])
	}

	if !found {
		return nil, dto.NotFound("view")
	}

	node.Views = newViews
	node.Modified = storage.Now()

	author := GitAuthor(user)
	if err := ws.WriteTable(ctx, node, false, author); err != nil {
		return nil, dto.InternalWithError("Failed to delete view", err)
	}

	return &dto.DeleteViewResponse{Ok: true}, nil
}

// --- DTO Conversions ---

func viewsToDTO(views []content.View) []dto.View {
	if views == nil {
		return nil
	}
	result := make([]dto.View, len(views))
	for i := range views {
		result[i] = viewToDTO(&views[i])
	}
	return result
}

func viewToDTO(v *content.View) dto.View {
	return dto.View{
		ID:      v.ID,
		Name:    v.Name,
		Type:    dto.ViewType(v.Type),
		Default: v.Default,
		Columns: viewColumnsToDTO(v.Columns),
		Filters: filtersToDTO(v.Filters),
		Sorts:   sortsToDTO(v.Sorts),
		Groups:  groupsToDTO(v.Groups),
	}
}

func viewColumnsToDTO(cols []content.ViewColumn) []dto.ViewColumn {
	if cols == nil {
		return nil
	}
	result := make([]dto.ViewColumn, len(cols))
	for i, c := range cols {
		result[i] = dto.ViewColumn{
			Property: c.Property,
			Width:    c.Width,
			Visible:  c.Visible,
		}
	}
	return result
}

func filtersToDTO(filters []content.Filter) []dto.Filter {
	if filters == nil {
		return nil
	}
	result := make([]dto.Filter, len(filters))
	for i, f := range filters {
		result[i] = filterToDTO(&f)
	}
	return result
}

func filterToDTO(f *content.Filter) dto.Filter {
	return dto.Filter{
		Property: f.Property,
		Operator: dto.FilterOp(f.Operator),
		Value:    f.Value,
		And:      filtersToDTO(f.And),
		Or:       filtersToDTO(f.Or),
	}
}

func sortsToDTO(sorts []content.Sort) []dto.Sort {
	if sorts == nil {
		return nil
	}
	result := make([]dto.Sort, len(sorts))
	for i, s := range sorts {
		result[i] = dto.Sort{
			Property:  s.Property,
			Direction: dto.SortDir(s.Direction),
		}
	}
	return result
}

func groupsToDTO(groups []content.Group) []dto.Group {
	if groups == nil {
		return nil
	}
	result := make([]dto.Group, len(groups))
	for i, g := range groups {
		result[i] = dto.Group{
			Property: g.Property,
			Hidden:   g.Hidden,
		}
	}
	return result
}

// --- Entity Conversions ---

func viewColumnsToEntity(cols []dto.ViewColumn) []content.ViewColumn {
	if cols == nil {
		return nil
	}
	result := make([]content.ViewColumn, len(cols))
	for i, c := range cols {
		result[i] = content.ViewColumn{
			Property: c.Property,
			Width:    c.Width,
			Visible:  c.Visible,
		}
	}
	return result
}

func filtersToEntity(filters []dto.Filter) []content.Filter {
	if filters == nil {
		return nil
	}
	result := make([]content.Filter, len(filters))
	for i, f := range filters {
		result[i] = filterToEntity(&f)
	}
	return result
}

func filterToEntity(f *dto.Filter) content.Filter {
	return content.Filter{
		Property: f.Property,
		Operator: content.FilterOp(f.Operator),
		Value:    f.Value,
		And:      filtersToEntity(f.And),
		Or:       filtersToEntity(f.Or),
	}
}

func sortsToEntity(sorts []dto.Sort) []content.Sort {
	if sorts == nil {
		return nil
	}
	result := make([]content.Sort, len(sorts))
	for i, s := range sorts {
		result[i] = content.Sort{
			Property:  s.Property,
			Direction: content.SortDir(s.Direction),
		}
	}
	return result
}

func groupsToEntity(groups []dto.Group) []content.Group {
	if groups == nil {
		return nil
	}
	result := make([]content.Group, len(groups))
	for i, g := range groups {
		result[i] = content.Group{
			Property: g.Property,
			Hidden:   g.Hidden,
		}
	}
	return result
}

// --- Schema Validation ---

// validateFiltersWithSchema validates filters against the table schema.
func validateFiltersWithSchema(filters []dto.Filter, props []content.Property) error {
	propSet := make(map[string]struct{}, len(props))
	for _, p := range props {
		propSet[p.Name] = struct{}{}
	}
	return validateFiltersRecursive(filters, propSet, 0, nil)
}

func validateFiltersRecursive(filters []dto.Filter, propSet map[string]struct{}, depth int, seen map[string]struct{}) error {
	if depth > 10 {
		return dto.InvalidField("filters", "nested filters exceed maximum depth")
	}
	if seen == nil {
		seen = make(map[string]struct{})
	}
	for _, f := range filters {
		if err := validateSingleFilter(&f, propSet); err != nil {
			return err
		}
		if f.Property != "" {
			if _, ok := seen[f.Property]; ok {
				return dto.InvalidField("filters", "duplicate property: "+f.Property)
			}
			seen[f.Property] = struct{}{}
		}
		// Nested And/Or get their own seen set (duplicates allowed across branches)
		if err := validateFiltersRecursive(f.And, propSet, depth+1, nil); err != nil {
			return err
		}
		if err := validateFiltersRecursive(f.Or, propSet, depth+1, nil); err != nil {
			return err
		}
	}
	return nil
}

func validateSingleFilter(f *dto.Filter, propSet map[string]struct{}) error {
	// Skip validation for pure grouping filters (only And/Or, no Property)
	if f.Property == "" && f.Operator == "" {
		return nil
	}

	if f.Property != "" {
		if _, ok := propSet[f.Property]; !ok {
			return dto.InvalidField("filters", "unknown property: "+f.Property)
		}
	}
	if f.Operator != "" {
		if err := f.Operator.Validate(); err != nil {
			return dto.InvalidField("operator", err.Error())
		}
	}
	// A leaf filter must have both property and operator
	if (f.Property != "") != (f.Operator != "") {
		if f.Property == "" {
			return dto.InvalidField("filters", "filter has operator but no property")
		}
		return dto.InvalidField("filters", "filter has property but no operator")
	}
	return nil
}

// validateSortsWithSchema validates sorts against the table schema.
func validateSortsWithSchema(sorts []dto.Sort, props []content.Property) error {
	propSet := make(map[string]struct{}, len(props))
	for _, p := range props {
		propSet[p.Name] = struct{}{}
	}
	seen := make(map[string]struct{})
	for _, s := range sorts {
		if s.Property == "" {
			return dto.InvalidField("sorts", "sort missing property")
		}
		if _, ok := propSet[s.Property]; !ok {
			return dto.InvalidField("sorts", "unknown property: "+s.Property)
		}
		if s.Direction != dto.SortAsc && s.Direction != dto.SortDesc {
			return dto.InvalidField("sorts", "invalid direction: "+string(s.Direction))
		}
		if _, ok := seen[s.Property]; ok {
			return dto.InvalidField("sorts", "duplicate property: "+s.Property)
		}
		seen[s.Property] = struct{}{}
	}
	return nil
}
