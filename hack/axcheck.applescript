-- Prints what the accessibility layer sees of the running Claimward app: the
-- menu-bar status item, and the native controls in the window. CI greps the
-- output (see .github/workflows/ci.yml, job launch). The app opens on the
-- Settings page when it has no configuration, which is where the text fields
-- and the provider pop-up button live.
tell application "System Events"
	tell process "claimward-app"
		set out to "status items: " & (count of menu bar items of menu bar 2) & linefeed
		set out to out & "windows: " & (count of windows) & linefeed
		set out to out & "top: " & (count of UI elements of window 1) & linefeed
		set out to out & "title: " & (name of window 1) & linefeed
		repeat with e in (entire contents of window 1)
			try
				set out to out & (role of e) & linefeed
			end try
		end repeat
	end tell
end tell
return out
