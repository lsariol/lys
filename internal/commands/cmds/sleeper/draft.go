package sleeper

import (
	"fmt"
	"time"
)

func TimeUntilDraft() string {
	location, _ := time.LoadLocation("America/New_York")
	draftTime := time.Date(2026, 9, 8, 17, 0, 0, 0, location)

	diff := time.Until(draftTime)
	if diff < 0 {
		diff = -diff

		days := int(diff.Hours()) / 24
		hours := int(diff.Hours()) % 24
		minutes := int(diff.Minutes()) % 60

		switch {
		case days > 0:
			return fmt.Sprintf(
				"Draft started %d days, %d hours, and %d minutes ago",
				days, hours, minutes,
			)

		case hours > 0:
			return fmt.Sprintf(
				"Draft started %d hours and %d minutes ago",
				hours, minutes,
			)

		default:
			return fmt.Sprintf(
				"Draft started %d minutes ago",
				minutes,
			)
		}
	}

	hours := int(diff.Hours())
	minutes := int(diff.Minutes()) % 60

	if hours > 0 {
		return fmt.Sprintf(
			"%d hours and %d minutes until the draft",
			hours, minutes,
		)
	}

	return fmt.Sprintf(
		"%d minutes until the draft",
		minutes,
	)
}
