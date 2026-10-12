// Схема «человечек»: фигура в центре (вид со спины — левая сторона тела слева), по бокам —
// трекеры с переключателями, от каждого линия к его месту на теле. Цвет линии — состояние трекера.
package main

import (
	"math"
	"sort"

	"github.com/diamondburned/gotk4/pkg/cairo"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

const (
	bodyWidth   = 560
	bodyHeight  = 630
	bodyColumn  = 172 // ширина колонки с трекерами
	bodyItemH   = 50
	bodyItemGap = 8
)

type point struct{ x, y float64 }

// Места трекеров на фигуре.
var bodyAnchors = map[string]point{
	"left_shoulder": {228, 142}, "right_shoulder": {332, 142},
	"chest": {280, 180}, "waist": {280, 318},
	"left_elbow": {206, 238}, "right_elbow": {354, 238},
	"left_knee": {252, 452}, "right_knee": {308, 452},
	"left_foot": {248, 572}, "right_foot": {312, 572},
}

var bodyColumns = [2][]string{
	{"left_shoulder", "chest", "left_elbow", "left_knee", "left_foot"},
	{"right_shoulder", "right_elbow", "waist", "right_knee", "right_foot"},
}

// Кости фигуры (ломаные).
var bodyFigure = [][]point{
	{{280, 108}, {280, 318}},                         // позвоночник
	{{228, 142}, {280, 126}, {332, 142}},             // плечи
	{{228, 142}, {206, 238}, {196, 330}},             // левая рука
	{{332, 142}, {354, 238}, {364, 330}},             // правая рука
	{{254, 326}, {280, 318}, {306, 326}},             // таз
	{{254, 326}, {252, 452}, {248, 572}, {242, 594}}, // левая нога
	{{306, 326}, {308, 452}, {312, 572}, {318, 594}}, // правая нога
}

var (
	colorTracked = [3]float64{0.18, 0.76, 0.49} // зелёный: в SteamVR и отслеживается
	colorWarning = [3]float64{0.90, 0.65, 0.04} // оранжевый: включён, но позиции нет
)

type bodyItem struct {
	name  string
	right bool
	y     float64 // центр строки
	sw    *gtk.Switch
	state *gtk.Label
	class string
}

type bodyDiagram struct {
	widget *gtk.Overlay
	area   *gtk.DrawingArea
	items  map[string]*bodyItem
}

func newBodyDiagram(onToggle func(), updating *bool) *bodyDiagram {
	d := &bodyDiagram{items: map[string]*bodyItem{}}
	d.area = gtk.NewDrawingArea()
	d.area.SetContentWidth(bodyWidth)
	d.area.SetContentHeight(bodyHeight)
	d.area.SetDrawFunc(d.draw)

	fixed := gtk.NewFixed()
	for side, names := range bodyColumns {
		// Строки — на высоте своих точек, но не налезая друг на друга.
		sorted := append([]string{}, names...)
		sort.Slice(sorted, func(i, j int) bool { return bodyAnchors[sorted[i]].y < bodyAnchors[sorted[j]].y })
		last := -math.MaxFloat64
		for _, name := range sorted {
			item := &bodyItem{name: name, right: side == 1, class: "dim-label"}
			item.y = math.Min(math.Max(bodyAnchors[name].y, last+bodyItemH+bodyItemGap), bodyHeight-bodyItemH/2)
			last = item.y
			x := 0.0
			if item.right {
				x = bodyWidth - bodyColumn
			}
			fixed.Put(d.newItemWidget(item, onToggle, updating), x, item.y-bodyItemH/2)
			d.items[name] = item
		}
	}

	d.widget = gtk.NewOverlay()
	d.widget.SetChild(d.area)
	d.widget.AddOverlay(fixed)
	d.widget.SetHAlign(gtk.AlignCenter)
	return d
}

// Строка трекера: название, состояние и переключатель; переключатель — со стороны фигуры.
func (d *bodyDiagram) newItemWidget(item *bodyItem, onToggle func(), updating *bool) gtk.Widgetter {
	box := gtk.NewBox(gtk.OrientationHorizontal, 10)
	box.SetSizeRequest(bodyColumn, bodyItemH)

	texts := gtk.NewBox(gtk.OrientationVertical, 0)
	texts.SetHExpand(true)
	texts.SetVAlign(gtk.AlignCenter)
	name := gtk.NewLabel(trackerLabel(item.name))
	name.AddCSSClass("heading")
	item.state = gtk.NewLabel("")
	item.state.AddCSSClass("caption")
	item.state.AddCSSClass("dim-label")
	for _, l := range []*gtk.Label{name, item.state} {
		l.SetWrap(true)
		if item.right {
			l.SetXAlign(0)
		} else {
			l.SetXAlign(1)
		}
		texts.Append(l)
	}

	item.sw = gtk.NewSwitch()
	item.sw.SetVAlign(gtk.AlignCenter)
	item.sw.NotifyProperty("active", func() {
		if !*updating {
			onToggle()
		}
	})
	if item.right {
		box.Append(item.sw)
		box.Append(texts)
	} else {
		box.Append(texts)
		box.Append(item.sw)
	}
	return box
}

func (d *bodyDiagram) setState(name, text, class string) {
	item := d.items[name]
	item.state.SetLabel(text)
	if item.class != class {
		item.class = class
		d.area.QueueDraw()
	}
}

func setColor(cr *cairo.Context, rgb [3]float64, alpha float64) {
	cr.SetSourceRGBA(rgb[0], rgb[1], rgb[2], alpha)
}

func (d *bodyDiagram) draw(area *gtk.DrawingArea, cr *cairo.Context, width, height int) {
	fg := area.Color()
	ink := [3]float64{float64(fg.Red()), float64(fg.Green()), float64(fg.Blue())}
	cr.Translate(float64(width-bodyWidth)/2, 0)
	cr.SetLineCap(cairo.LineCapRound)
	cr.SetLineJoin(cairo.LineJoinRound)

	// Фигура.
	setColor(cr, ink, 0.16)
	cr.SetLineWidth(16)
	for _, bone := range bodyFigure {
		cr.MoveTo(bone[0].x, bone[0].y)
		for _, p := range bone[1:] {
			cr.LineTo(p.x, p.y)
		}
		cr.Stroke()
	}
	cr.Arc(280, 72, 28, 0, 2*math.Pi)
	cr.Fill()

	// Линии от строк к точкам на теле.
	for _, item := range d.items {
		color, alpha := ink, 0.28
		switch item.class {
		case "success":
			color, alpha = colorTracked, 1
		case "warning":
			color, alpha = colorWarning, 1
		}
		anchor := bodyAnchors[item.name]
		x := float64(bodyColumn) + 4
		if item.right {
			x = bodyWidth - bodyColumn - 4
		}
		setColor(cr, color, alpha)
		cr.SetLineWidth(2)
		cr.MoveTo(x, item.y)
		cr.LineTo(anchor.x, anchor.y)
		cr.Stroke()
		cr.Arc(x, item.y, 3, 0, 2*math.Pi)
		cr.Fill()

		cr.Arc(anchor.x, anchor.y, 8, 0, 2*math.Pi)
		if item.class == "dim-label" {
			cr.SetLineWidth(2)
			cr.Stroke()
		} else {
			cr.Fill()
		}
	}

	setColor(cr, ink, 0.4)
	cr.SelectFontFace("sans-serif", cairo.FontSlantNormal, cairo.FontWeightNormal)
	cr.SetFontSize(11)
	caption := T("вид со спины")
	extents := cr.TextExtents(caption)
	cr.MoveTo(280-extents.Width/2, bodyHeight-6)
	cr.ShowText(caption)
}

func trackerLabel(name string) string {
	for _, t := range trackers {
		if t.Name == name {
			return T(t.Label)
		}
	}
	return name
}
