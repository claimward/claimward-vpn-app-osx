-- Prints what the accessibility layer sees of the running Claimward app: the
-- menu-bar status item and the window, including the buttons go-widgets paints
-- (View.A11yElements publishes them). CI greps the output (see
-- .github/workflows/ci.yml, job launch). It only reads; it never clicks.
--
-- hack/axpress.applescript is the other half: it presses the painted
-- "Settings" button through AXPress and checks the page changed.
tell application "System Events"
	tell process "claimward-app"
		set out to "status items: " & (count of menu bar items of menu bar 2) & linefeed
		set out to out & "windows: " & (count of windows) & linefeed
		set out to out & "title: " & (name of window 1) & linefeed
		set found to false
		repeat with e in (entire contents of window 1)
			try
				if (role of e) is "AXButton" and (description of e) is "Settings" then set found to true
			end try
		end repeat
		set out to out & "settings button: " & found & linefeed
	end tell
end tell
return out
