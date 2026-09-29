package main

import "time"

const (
	floatingDockSnap  = 16
	floatingDockStrip = 4
	floatingDockHover = 8
	floatingDockDelay = 600 * time.Millisecond
)

type dockEdge uint8

const (
	dockNone dockEdge = iota
	dockLeft
	dockRight
	dockTop
	dockBottom
)

type desktopRect struct{ Left, Top, Right, Bottom int }

func (r desktopRect) width() int  { return r.Right - r.Left }
func (r desktopRect) height() int { return r.Bottom - r.Top }
func (r desktopRect) contains(x, y int) bool {
	return x >= r.Left && x < r.Right && y >= r.Top && y < r.Bottom
}

func clampFloatingRect(r, work desktopRect) desktopRect {
	w, h := r.width(), r.height()
	x := max(work.Left, min(r.Left, work.Right-w))
	y := max(work.Top, min(r.Top, work.Bottom-h))
	return desktopRect{x, y, x + w, y + h}
}

func snapFloatingRect(r, work desktopRect) (desktopRect, dockEdge) {
	r = clampFloatingRect(r, work)
	edge, best := dockNone, floatingDockSnap+1
	for _, candidate := range []struct {
		edge     dockEdge
		distance int
	}{
		{dockLeft, r.Left - work.Left}, {dockRight, work.Right - r.Right},
		{dockTop, r.Top - work.Top}, {dockBottom, work.Bottom - r.Bottom},
	} {
		if candidate.distance >= 0 && candidate.distance < best {
			edge, best = candidate.edge, candidate.distance
		}
	}
	w, h := r.width(), r.height()
	switch edge {
	case dockLeft:
		r.Left = work.Left
		r.Right = r.Left + w
	case dockRight:
		r.Right = work.Right
		r.Left = r.Right - w
	case dockTop:
		r.Top = work.Top
		r.Bottom = r.Top + h
	case dockBottom:
		r.Bottom = work.Bottom
		r.Top = r.Bottom - h
	}
	return r, edge
}

func collapsedFloatingRect(r desktopRect, edge dockEdge) desktopRect {
	switch edge {
	case dockLeft:
		r.Right = r.Left + floatingDockStrip
	case dockRight:
		r.Left = r.Right - floatingDockStrip
	case dockTop:
		r.Bottom = r.Top + floatingDockStrip
	case dockBottom:
		r.Top = r.Bottom - floatingDockStrip
	}
	return r
}

type floatingDock struct {
	normal, work        desktopRect
	edge                dockEdge
	collapsed, dragging bool
	outsideSince        time.Time
}

func (d *floatingDock) place(r, work desktopRect, autoHide bool) {
	d.normal, d.work = clampFloatingRect(r, work), work
	d.edge = dockNone
	if autoHide {
		d.normal, d.edge = snapFloatingRect(r, work)
	}
	d.collapsed = false
	d.outsideSince = time.Time{}
}

func (d *floatingDock) visibleRect() desktopRect {
	if d.collapsed {
		return collapsedFloatingRect(d.normal, d.edge)
	}
	return d.normal
}

func (d *floatingDock) nearEdge(x, y int) bool {
	if d.edge == dockNone || !d.work.contains(x, y) {
		return false
	}
	r := collapsedFloatingRect(d.normal, d.edge)
	r.Left -= floatingDockHover
	r.Right += floatingDockHover
	r.Top -= floatingDockHover
	r.Bottom += floatingDockHover
	return r.contains(x, y)
}

// Manual hiding is handled by the window adapter before calling this method.
func (d *floatingDock) update(now time.Time, x, y int) bool {
	if d.dragging || d.edge == dockNone {
		d.outsideSince = time.Time{}
		return false
	}
	if d.collapsed {
		if d.nearEdge(x, y) {
			d.collapsed = false
			d.outsideSince = time.Time{}
			return true
		}
		return false
	}
	if d.normal.contains(x, y) || d.nearEdge(x, y) {
		d.outsideSince = time.Time{}
		return false
	}
	if d.outsideSince.IsZero() || now.Before(d.outsideSince) {
		d.outsideSince = now
		return false
	}
	if now.Sub(d.outsideSince) >= floatingDockDelay {
		d.collapsed = true
		d.outsideSince = time.Time{}
		return true
	}
	return false
}
