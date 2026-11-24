-- Presses the painted "Settings" button through the accessibility API (AXPress),
-- then reports how many text fields the window shows: the Settings page has the
-- native ones, the status page none. Proves a painted widget is both in the AX
-- tree and operable from it.
tell application "System Events"
	tell process "claimward-app"
		set els to entire contents of window 1
		set nBefore to 0
		set target to missing value
		repeat with e in els
			try
				if (role of e) is "AXTextField" then set nBefore to nBefore + 1
			end try
			try
				if (role of e) is "AXButton" and (description of e) is "Settings" then set target to e
			end try
		end repeat
		if target is missing value then return "no Settings button in the AX tree"
		perform action "AXPress" of target
		delay 1.5
		set nAfter to 0
		repeat with e in (entire contents of window 1)
			try
				if (role of e) is "AXTextField" then set nAfter to nAfter + 1
			end try
		end repeat
		return "pressed Settings" & linefeed & "text fields before: " & nBefore & linefeed & "text fields after: " & nAfter
	end tell
end tell
