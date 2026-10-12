// Вкладка «3D-вид»: скелет со шлема линиями, включённые трекеры — крупными точками с подписями.
// Камера по умолчанию стоит за спиной человека и следует за его поворотом; перетаскивание мышью
// поворачивает её вокруг человека, колесо — масштаб. Координаты — STAGE шлема (y вверх, пол = 0).
package main

import (
	"math"
	"strings"

	"github.com/diamondburned/gotk4/pkg/cairo"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// Кости скелета по индексам XrBodyJointBD (common/protocol.h).
var skeletonBones = [][2]int{
	{0, 3}, {3, 6}, {6, 9}, {9, 12}, {12, 15}, // таз → позвоночник → шея → голова
	{0, 1}, {1, 4}, {4, 7}, {7, 10}, // левая нога
	{0, 2}, {2, 5}, {5, 8}, {8, 11}, // правая нога
	{9, 13}, {13, 16}, {16, 18}, {18, 20}, {20, 22}, // левая рука
	{9, 14}, {14, 17}, {17, 19}, {19, 21}, {21, 23}, // правая рука
}

// Индекс сустава для каждого трекера (driver/src/driver.cpp, kTrackerSpecs).
var trackerJoints = map[string]int{
	"waist": 0, "chest": 9, "left_foot": 7, "right_foot": 8, "left_knee": 4, "right_knee": 5,
	"left_elbow": 18, "right_elbow": 19, "left_shoulder": 16, "right_shoulder": 17,
	"left_hip": 1, "right_hip": 2, "spine1": 3, "spine2": 6, "left_toes": 10, "right_toes": 11,
	"neck": 12, "left_collar": 13, "right_collar": 14, "head": 15,
	"left_wrist": 20, "right_wrist": 21, "left_hand": 22, "right_hand": 23,
}

type view3D struct {
	widget  *gtk.Box
	area    *gtk.DrawingArea
	c       *Controller
	enabled func() []string // включённые трекеры (из переключателей)

	bodyYaw              float64 // куда повёрнут человек (сглаженно)
	yaw, pitch, distance float64 // поворот камеры относительно «за спиной»
	dragYaw, dragPitch   float64
}

func newView3D(c *Controller, enabled func() []string) *view3D {
	v := &view3D{c: c, enabled: enabled, pitch: 0.35, distance: 2.8}
	v.area = gtk.NewDrawingArea()
	v.area.SetVExpand(true)
	v.area.SetHExpand(true)
	v.area.SetDrawFunc(v.draw)

	drag := gtk.NewGestureDrag()
	drag.ConnectDragBegin(func(x, y float64) { v.dragYaw, v.dragPitch = v.yaw, v.pitch })
	drag.ConnectDragUpdate(func(dx, dy float64) {
		v.yaw = v.dragYaw + dx*0.01
		v.pitch = math.Max(-1.4, math.Min(1.4, v.dragPitch+dy*0.01))
		v.area.QueueDraw()
	})
	v.area.AddController(drag)
	scroll := gtk.NewEventControllerScroll(gtk.EventControllerScrollVertical)
	scroll.ConnectScroll(func(dx, dy float64) bool {
		v.distance = math.Max(1, math.Min(12, v.distance*math.Pow(1.1, dy)))
		v.area.QueueDraw()
		return true
	})
	v.area.AddController(scroll)

	hint := gtk.NewLabel(T("Камера за спиной и следует за поворотом. Перетаскивайте мышью — облёт, колесо — масштаб."))
	hint.AddCSSClass("dim-label")
	hint.AddCSSClass("caption")
	hint.SetMarginTop(6)
	hint.SetMarginBottom(10)

	v.widget = gtk.NewBox(gtk.OrientationVertical, 0)
	v.widget.Append(v.area)
	v.widget.Append(hint)
	return v
}

func (v *view3D) draw(area *gtk.DrawingArea, cr *cairo.Context, width, height int) {
	fg := area.Color()
	ink := [3]float64{float64(fg.Red()), float64(fg.Green()), float64(fg.Blue())}
	w, h := float64(width), float64(height)
	poses, fresh := v.c.Poses()
	cr.SetLineCap(cairo.LineCapRound)
	cr.SelectFontFace("sans-serif", cairo.FontSlantNormal, cairo.FontWeightNormal)

	if !fresh {
		setColor(cr, ink, 0.5)
		cr.SetFontSize(15)
		text := T("Нет данных — 3D-вид оживёт, когда пойдёт трекинг")
		extents := cr.TextExtents(text)
		cr.MoveTo((w-extents.Width)/2, h/2)
		cr.ShowText(text)
		return
	}

	// Направление «вправо» у человека — по линии бёдер (или плеч); камера встаёт за спину:
	// после поворота на угол yaw правая сторона тела смотрит вправо экрана, лицо — от камеры.
	if left, right := poses[1], poses[2]; left.Valid && right.Valid {
		target := math.Atan2(right.Z-left.Z, right.X-left.X)
		v.bodyYaw += math.Remainder(target-v.bodyYaw, 2*math.Pi) * 0.15
	}
	// Камера смотрит на таз по горизонтали и на середину роста по вертикали.
	tx, tz, ty := poses[0].X, poses[0].Z, 0.85
	if poses[15].Valid {
		ty = poses[15].Y / 2
	}
	focal := math.Min(w, h) * 1.2
	yaw := v.bodyYaw + v.yaw
	project := func(x, y, z float64) (float64, float64, bool) {
		x, y, z = x-tx, y-ty, z-tz
		cy, sy := math.Cos(yaw), math.Sin(yaw)
		x, z = cy*x+sy*z, -sy*x+cy*z
		cp, sp := math.Cos(v.pitch), math.Sin(v.pitch)
		y, z = cp*y-sp*z, sp*y+cp*z
		depth := v.distance - z // камера на +z, смотрит в сторону −z (правая система координат)
		if depth < 0.1 {
			return 0, 0, false
		}
		return w/2 + focal*x/depth, h/2 - focal*y/depth, true
	}
	line := func(a, b [3]float64) {
		x1, y1, ok1 := project(a[0], a[1], a[2])
		x2, y2, ok2 := project(b[0], b[1], b[2])
		if ok1 && ok2 {
			cr.MoveTo(x1, y1)
			cr.LineTo(x2, y2)
		}
	}

	// Пол: сетка 4×4 м с шагом 0.5 м вокруг человека.
	setColor(cr, ink, 0.2)
	cr.SetLineWidth(1)
	for i := -4; i <= 4; i++ {
		d := float64(i) * 0.5
		line([3]float64{tx - 2, 0, tz + d}, [3]float64{tx + 2, 0, tz + d})
		line([3]float64{tx + d, 0, tz - 2}, [3]float64{tx + d, 0, tz + 2})
	}
	cr.Stroke()

	// Скелет.
	setColor(cr, ink, 0.55)
	cr.SetLineWidth(3)
	for _, bone := range skeletonBones {
		a, b := poses[bone[0]], poses[bone[1]]
		if a.Valid && b.Valid {
			line([3]float64{a.X, a.Y, a.Z}, [3]float64{b.X, b.Y, b.Z})
		}
	}
	cr.Stroke()

	// Включённые трекеры.
	cr.SetFontSize(12)
	for _, name := range v.enabled() {
		joint, ok := trackerJoints[name]
		if !ok {
			continue
		}
		p := poses[joint]
		x, y, visible := project(p.X, p.Y, p.Z)
		if !visible {
			continue
		}
		color := colorTracked
		if !p.Valid {
			color = colorWarning
		}
		setColor(cr, color, 1)
		cr.Arc(x, y, 7, 0, 2*math.Pi)
		cr.Fill()
		// Подписи левой стороны — левее точки, остальные — правее, чтобы не лезть на тело.
		label := trackerLabel(name)
		lx := x + 11
		if strings.HasPrefix(name, "left_") {
			lx = x - 11 - cr.TextExtents(label).Width
		}
		setColor(cr, ink, 0.85)
		cr.MoveTo(lx, y+4)
		cr.ShowText(label)
	}
}
