package calendar

// SelectMsg tells the parent that the cursor landed on another day, for
// example to list that day's contributions. The calendar sends it whenever a
// key moves the cursor to a different day.
type SelectMsg struct {
	// ID is the ID of the calendar that sent the message.
	ID int
	// Day is the day under the cursor.
	Day Day
}
