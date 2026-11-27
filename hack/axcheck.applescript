-- Prints what the accessibility layer sees of the running Claimward app: the
-- menu-bar status item and the window, including the buttons go-widgets paints
-- (View.A11yElements publishes them). CI greps the output (see
-- .github/workflows/ci.yml, job launch). It only reads; it never clicks.
--
-- It looks for the servers list's "Add" button (the + of the sidebar).
-- hack/axpress.applescript is the other half: it presses the painted
-- "Settings" button through AXPress and checks the page changed.
tell application "System Events"
	tell process "claimward-app"
		set out to "status items: " & (count of menu bar items of menu bar 2) & linefeed
		set out to out & "windows: " & (count of windows) & linefeed
		set out to out & "title: " & (name of window 1) & linefeed
		set found to false
		set els to entire contents of window 1
		repeat with e in els
			set r to ""
			set labels to {}
			try
				set r to (role of e)
			end try
			try
				set end of labels to (description of e)
			end try
			try
				set end of labels to (title of e)
			end try
			try
				set end of labels to (name of e)
			end try
			if r is "AXButton" and labels contains "Add" then set found to true
			set out to out & "element: " & r & " | " & (labels as text) & linefeed
		end repeat
		set out to out & "add button: " & found & linefeed
	end tell
end tell
return out
