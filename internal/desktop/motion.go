package desktop

import (
	"time"

	"github.com/egoist/mygo/ui"
)

const (
	railOpenDuration     = 260 * time.Millisecond
	railCloseDuration    = 190 * time.Millisecond
	popoverEnterDuration = 150 * time.Millisecond
	popoverExitDuration  = 100 * time.Millisecond
	dialogEnterDuration  = 180 * time.Millisecond
	dialogExitDuration   = 120 * time.Millisecond
	stateColorDuration   = 120 * time.Millisecond
)

func railAnimation(open bool) ui.Animation {
	if open {
		return ui.Animation{Duration: railOpenDuration, Ease: ui.EaseOut}
	}
	return ui.Animation{Duration: railCloseDuration, Ease: ui.EaseInOut}
}

func popoverTransition() ui.ElementTransition {
	return ui.ElementTransition{
		Duration:     popoverEnterDuration,
		Ease:         ui.EaseOut,
		ExitDuration: popoverExitDuration,
		ExitEase:     ui.EaseIn,
		Enter:        &ui.Motion{Y: -6, Opacity: 0, Flip: true},
		Exit:         &ui.Motion{Y: -6, Opacity: 0, Flip: true},
	}
}

func dialogBackdropTransition() ui.ElementTransition {
	return ui.ElementTransition{
		Duration:     dialogEnterDuration,
		Ease:         ui.EaseOut,
		ExitDuration: dialogExitDuration,
		ExitEase:     ui.EaseIn,
		Enter:        &ui.Motion{Opacity: 0},
		Exit:         &ui.Motion{Opacity: 0},
	}
}

func stateColorAnimation() ui.Animation {
	return ui.Animation{Duration: stateColorDuration, Ease: ui.EaseOut, NonMotion: true}
}
