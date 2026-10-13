# Records demo/envy.gif with https://github.com/charmbracelet/vhs.
# Run from the repository root:
#   vhs demo/demo.tape
# It builds envy and works on a temporary copy of the fixtures, so the
# repository is left untouched.

Output demo/envy.gif

Set Shell "bash"
Set FontSize 18
Set Width 1400
Set Height 760
Set Padding 24
Set TypingSpeed 45ms
Set Theme "Catppuccin Mocha"

Hide
Type `D=$(mktemp -d) && cp demo/.env demo/.env.example "$D" && go build -o "$D/bin/envy" . && export PATH="$D/bin:$PATH" && cd "$D" && export PS1='$ ' && clear`
Enter
Sleep 8s
Show

Type "envy audit"
Sleep 400ms
Enter
Sleep 5s

Type "clear && envy diff .env .env.example"
Sleep 400ms
Enter
Sleep 2s

# Walk down to the leaking key (row 6), reveal values, then mask them again.
Down 5
Sleep 2500ms
Type "m"
Sleep 2s
Type "m"
Sleep 1s

# Copy REDIS_URL from the template into .env and save.
Up
Sleep 600ms
Tab
Sleep 600ms
Space
Sleep 1500ms
Type "s"
Sleep 1500ms
Type "q"
Sleep 1s

Type "clear && envy run -- sh -c 'echo listening on :$PORT'"
Sleep 400ms
Enter
Sleep 2s
