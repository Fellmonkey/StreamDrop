package ws

// SetSnapshotBuilder replaces the snapshot provider after construction. The
// application builds its container in dependency order, so the snapshot can
// only be defined once every subsystem exists.
func (h *Hub) SetSnapshotBuilder(fn func() any) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.snapshot = fn
}
