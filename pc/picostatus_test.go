package main

import (
	"encoding/json"
	"testing"
)

// Заряд трекеров PICO одной строкой: по местам (левая, правая, пояс), без мест — просто проценты (драйвер 1.7).
func TestPicoBatteryText(t *testing.T) {
	defer func(was bool) { english = was }(english)
	english = false
	for _, c := range []struct {
		json, text string
		low        bool
	}{
		{`{"trackers":[{"id":1,"place":2,"battery":80},{"id":2,"place":1,"battery":15}]}`,
			"заряд: левая нога 15 %, правая нога 80 %", true},
		{`{"trackers":[{"id":1,"place":2,"battery":80},{"id":3,"place":3,"battery":60},{"id":2,"place":1,"battery":70}]}`,
			"заряд: левая нога 70 %, правая нога 80 %, пояс 60 %", false},
		{`{"trackers":[{"id":1,"battery":80},{"id":2,"battery":60}]}`, "заряд: 80 %, 60 %", false},
		{`{"trackers":[{"id":1,"place":2,"battery":-1}]}`, "", false},
	} {
		var p PicoStatus
		if err := json.Unmarshal([]byte(c.json), &p); err != nil {
			t.Fatal(err)
		}
		if text, low := p.BatteryText(); text != c.text || low != c.low {
			t.Errorf("%s: %q, %v; ждал %q, %v", c.json, text, low, c.text, c.low)
		}
	}
	var none *PicoStatus
	if text, _ := none.BatteryText(); text != "" {
		t.Errorf("nil: %q", text)
	}
	english = true
	var p PicoStatus
	json.Unmarshal([]byte(`{"trackers":[{"id":1,"place":2,"battery":80},{"id":2,"place":1,"battery":60}]}`), &p)
	if text, _ := p.BatteryText(); text != "battery: left leg 60%, right leg 80%" {
		t.Errorf("английский: %q", text)
	}
}
