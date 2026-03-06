package app

type Theme struct {
	Name          string
	Background    string
	Foreground    string
	Accent        string
	CodeBG        string
	Border        string
	BorderFocused string
	Muted         string
}

var builtInThemes = []Theme{
	{Name: "dracula", Background: "#282a36", Foreground: "#f8f8f2", Accent: "#bd93f9", CodeBG: "#44475a", Border: "#6272a4", BorderFocused: "#ff79c6", Muted: "#a4acc4"},
	{Name: "nord", Background: "#2e3440", Foreground: "#eceff4", Accent: "#88c0d0", CodeBG: "#3b4252", Border: "#4c566a", BorderFocused: "#88c0d0", Muted: "#81a1c1"},
	{Name: "gruvbox", Background: "#282828", Foreground: "#ebdbb2", Accent: "#fabd2f", CodeBG: "#3c3836", Border: "#665c54", BorderFocused: "#fe8019", Muted: "#a89984"},
	{Name: "light", Background: "#fdf6e3", Foreground: "#657b83", Accent: "#268bd2", CodeBG: "#eee8d5", Border: "#93a1a1", BorderFocused: "#268bd2", Muted: "#93a1a1"},
	{Name: "tokyo-night", Background: "#1a1b26", Foreground: "#c0caf5", Accent: "#7aa2f7", CodeBG: "#24283b", Border: "#414868", BorderFocused: "#7dcfff", Muted: "#7a88cf"},
}

func getTheme(name string) Theme {
	for _, theme := range builtInThemes {
		if theme.Name == name {
			return theme
		}
	}
	return builtInThemes[0]
}

func nextTheme(name string) Theme {
	for index, theme := range builtInThemes {
		if theme.Name == name {
			return builtInThemes[(index+1)%len(builtInThemes)]
		}
	}
	return builtInThemes[0]
}
