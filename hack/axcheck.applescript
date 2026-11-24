-- Prints what the accessibility layer sees of the running Claimward app: the
-- menu-bar status item and the window. CI greps the output (see
-- .github/workflows/ci.yml, job launch). It only reads; it never clicks.
--
-- The native text fields and the provider pop-up button live on the Settings
-- page, whose "Settings" button is painted by go-widgets and so is not in the
-- accessibility tree: reaching it would need a click by position, which this
-- check deliberately does not do (a stray click lands on whatever is under it).
-- Those controls are asserted by internal/ui TestNativeControlsAreOneControlPerShownInput.
tell application "System Events"
	tell process "claimward-app"
		set out to "status items: " & (count of menu bar items of menu bar 2) & linefeed
		set out to out & "windows: " & (count of windows) & linefeed
		set out to out & "title: " & (name of window 1) & linefeed
	end tell
end tell
return out
