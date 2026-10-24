package main

import (
	"sort"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var blockPalette = []choice{{"#ef4444", "Red"}, {"#881337", "Burgundy"}, {"#fb7185", "Coral"}, {"#f97316", "Orange"}, {"#f59e0b", "Amber"}, {"#eab308", "Yellow"}, {"#92400e", "Brown"}, {"#808000", "Olive"}, {"#84cc16", "Lime"}, {"#22c55e", "Green"}, {"#10b981", "Emerald"}, {"#14b8a6", "Teal"}, {"#06b6d4", "Cyan"}, {"#0ea5e9", "Sky"}, {"#3b82f6", "Blue"}, {"#1e3a8a", "Navy"}, {"#6366f1", "Indigo"}, {"#8b5cf6", "Violet"}, {"#a855f7", "Purple"}, {"#d946ef", "Magenta"}, {"#ec4899", "Pink"}, {"#ffffff", "White"}, {"#c0c0c0", "Silver"}, {"#808080", "Gray"}, {"#000000", "Black"}}

func paletteOptions(brackets bool) []choice {
	options := []choice{{"custom", "Custom hex…"}}
	if brackets {
		options = append(options, choice{"", "Automatic contrast"}, choice{"same", "Automatic same color"}, choice{"none", "None"})
	} else {
		options = append(options, choice{"", "Clear color"})
	}
	for _, c := range blockPalette {
		if brackets && !strings.Contains("|Red|Orange|Yellow|Green|Cyan|Blue|Purple|Pink|White|Gray|Black|", "|"+c.Label+"|") {
			continue
		}
		options = append(options, c)
	}
	return options
}

func colorName(color string) string {
	if color == "same" {
		return "Automatic same color"
	}
	if color == "none" {
		return "None"
	}
	if color == "" {
		return "None"
	}
	for _, c := range blockPalette {
		if strings.EqualFold(color, c.ID) {
			return c.Label
		}
	}
	return color
}

func editHostColors(block, bracket string, save ...func(string, string) error) (string, string, bool, error) {
	defer menuSession()()
	originalBlock, originalBracket := block, bracket
	for {
		bracketName := colorName(bracket)
		if bracket == "" {
			bracketName = "Automatic contrast"
		}
		m := newDialog("Host color", []choice{{"block", "Block color: " + colorName(block)}, {"bracket", "Bracket color: " + bracketName}}, "Preview: "+bracketName+" brackets · "+colorName(block)+" block.\n\nSave writes only colors to inventory, leaving other host edits unsaved. Clear affects the highlighted field only; Save is still required. Back discards unsaved color changes.")
		m.swatch, m.swatchBracket = block, bracket
		if block == "Mixed" {
			m.swatch = ""
		}
		if bracket == "Mixed" {
			m.swatchBracket = ""
		}
		m.footerActions = []choice{{"save", "Save"}, {"clear", "Clear selected"}}
		m.optionDetails = map[string]string{"block": "Choose the square's color from 25 presets or a custom hex value.", "bracket": "Automatic contrast chooses light/dark brackets; Automatic same color follows the block; None hides brackets. Delete sets None. Preset/custom colors stay independent. Mixed brackets remain unchanged unless explicitly selected."}
		id, back, err := runChoice(m, "host-color-editor")
		if err != nil || back {
			return originalBlock, originalBracket, true, err
		}
		switch id {
		case "save":
			if len(save) > 0 {
				if err = save[0](block, bracket); err != nil {
					if e := notice(err.Error()); e != nil {
						return originalBlock, originalBracket, true, e
					}
					continue
				}
			}
			return block, bracket, false, nil
		case "clear":
			if len(m.filtered) > 0 {
				if m.options[m.filtered[m.cursor]].ID == "bracket" {
					bracket = "none"
				} else {
					block = ""
				}
			}
		case "block", "bracket":
			current := block
			if id == "bracket" {
				current = bracket
			}
			if current == "Mixed" {
				current = ""
			}
			value, b, e := choosePaletteColor(current, id == "bracket")
			if e != nil {
				return originalBlock, originalBracket, true, e
			}
			if !b {
				if id == "block" {
					block = value
				} else {
					bracket = value
				}
			}
		}
	}
}

// Hue puts chromatic colors first and groups neutrals at the end.
func paletteHue(color string) float64 {
	n, _ := strconv.ParseUint(strings.TrimPrefix(color, "#"), 16, 24)
	r, g, b := float64(n>>16), float64((n>>8)&255), float64(n&255)
	hi, lo := max(r, g, b), min(r, g, b)
	if hi == lo {
		return 360 + (255-hi)/255
	}
	h := (g - b) / (hi - lo)
	if hi == g {
		h = 2 + (b-r)/(hi-lo)
	} else if hi == b {
		h = 4 + (r-g)/(hi-lo)
	}
	if h < 0 {
		h += 6
	}
	return h * 60
}

func (m *dialogModel) sortPalette() {
	if !m.palette {
		return
	}
	sort.SliceStable(m.filtered, func(i, j int) bool {
		a, b := m.options[m.filtered[i]], m.options[m.filtered[j]]
		ac, bc := hostHexColor.MatchString(a.ID), hostHexColor.MatchString(b.ID)
		if !ac || !bc {
			if ac != bc {
				return !ac
			}
			return m.filtered[i] < m.filtered[j]
		}
		cmp := 0
		if m.paletteSort.Column == 0 {
			ah, bh := paletteHue(a.ID), paletteHue(b.ID)
			if ah < bh {
				cmp = -1
			} else if ah > bh {
				cmp = 1
			}
		} else if m.paletteSort.Column == 1 {
			cmp = strings.Compare(strings.ToLower(a.Label), strings.ToLower(b.Label))
		} else {
			cmp = strings.Compare(a.ID, b.ID)
		}
		if cmp == 0 {
			return a.ID < b.ID
		}
		if m.paletteSort.Descending {
			return cmp > 0
		}
		return cmp < 0
	})
}

func (m *dialogModel) paletteWidths(width int) []int {
	a, b := 8, 12
	if m.layout.labelWidth > 0 {
		a = m.layout.labelWidth
	}
	if m.layout.typeWidth > 0 {
		b = m.layout.typeWidth
	}
	a = min(max(4, a), max(4, width-12))
	b = min(max(4, b), max(4, width-a-8))
	return []int{a, b, max(1, width-a-b-8)}
}

func (m *dialogModel) paletteLine(values []string, width int) string {
	var cells []string
	for i, w := range m.paletteWidths(width) {
		v := fit(values[i], w)
		cells = append(cells, v+strings.Repeat(" ", max(0, w-lipgloss.Width(v))))
	}
	return fit(strings.Join(cells, menuColumnSeparator()), width)
}
