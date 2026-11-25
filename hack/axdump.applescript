-- Prints the role, description, title and name of every element of the
-- Claimward window, as System Events sees them.
tell application "System Events"
	tell process "claimward-app"
		set els to entire contents of window 1
		set out to "elements: " & (count of els) & linefeed
		repeat with e in els
			set txt to ""
			try
				set txt to txt & (role of e)
			end try
			try
				set txt to txt & " | " & (description of e)
			end try
			try
				set txt to txt & " | " & (title of e)
			end try
			try
				set txt to txt & " | " & (name of e)
			end try
			set out to out & txt & linefeed
		end repeat
	end tell
end tell
return out
