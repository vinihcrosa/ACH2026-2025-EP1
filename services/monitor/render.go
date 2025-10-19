package main

import (
	"fmt"
	"math"
	"strings"

	"github.com/mattn/go-runewidth"
)

// truncate limita o tamanho de uma string preservando espaçamento da lista.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

// min devolve o menor entre dois inteiros.
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// max devolve o maior entre dois inteiros.
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// humanBytes formata valores em bytes para uma representação amigável.
func humanBytes(v uint64) string {
	const unit = 1024
	if v < unit {
		return fmt.Sprintf("%dB", v)
	}
	div, exp := uint64(unit), 0
	for n := v / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	value := float64(v) / float64(div)
	return fmt.Sprintf("%.2f%cB", value, "KMGTPE"[exp])
}

// coloredBar cria uma barra horizontal colorida para uso em textos.
func coloredBar(value float64, width int) string {
	if width <= 0 {
		width = 1
	}
	if value < 0 {
		value = 0
	}
	if value > 100 {
		value = 100
	}

	filled := int(math.Round(value / 100 * float64(width)))
	if filled > width {
		filled = width
	}
	if filled < 0 {
		filled = 0
	}

	color := colorForUsage(value)
	var b strings.Builder
	if filled > 0 {
		b.WriteString("[")
		b.WriteString(color)
		b.WriteString("]")
		b.WriteString(strings.Repeat("█", filled))
		b.WriteString("[-]")
	}
	if width-filled > 0 {
		b.WriteString("[#3b3b3b]")
		b.WriteString(strings.Repeat("░", width-filled))
		b.WriteString("[-]")
	}
	return b.String()
}

// colorForUsage determina uma cor baseada em faixas de utilização.
func colorForUsage(value float64) string {
	switch {
	case value >= 90:
		return "#ff5555"
	case value >= 75:
		return "#ffb86c"
	case value >= 50:
		return "#f1fa8c"
	case value >= 25:
		return "#50fa7b"
	default:
		return "#8be9fd"
	}
}

// labelledHeatmapLines cria linhas com mapa de calor e título alinhado.
func labelledHeatmapLines(title string, values []float64, width, height int) []string {
	lines := renderHeatmapLines(values, width, height)
	if len(lines) == 0 {
		return nil
	}

	label := fmt.Sprintf("[yellow]%s[-]", title)
	lines[0] = padMarkup(label, visibleWidth(lines[0]))
	return lines
}

// renderHeatmapLines converte uma série em blocos coloridos estilo heatmap.
func renderHeatmapLines(values []float64, width, height int) []string {
	if len(values) == 0 || height <= 0 {
		return nil
	}
	if width <= 0 {
		width = len(values)
	}
	if len(values) > width {
		values = values[len(values)-width:]
	} else if len(values) < width {
		padding := make([]float64, width-len(values))
		values = append(padding, values...)
	}
	if height < 2 {
		height = 2
	}

	lines := make([]string, height)
	for row := height - 1; row >= 0; row-- {
		var line strings.Builder
		for _, v := range values {
			filled := int(math.Round(v / 100 * float64(height-1)))
			if filled > height-1 {
				filled = height - 1
			}
			if filled < 0 {
				filled = 0
			}
			if filled >= row {
				color := colorForUsage(v)
				line.WriteString("[")
				line.WriteString(color)
				line.WriteString("]•[-]")
			} else {
				line.WriteString(" ")
			}
		}
		lines[height-1-row] = line.String()
	}
	return lines
}

// mergeColumns alinha duas colunas de texto, aplicando espaçamento entre elas.
func mergeColumns(left []string, right []string, gap string) string {
	if len(left) == 0 && len(right) == 0 {
		return ""
	}

	maxWidth := 0
	for _, line := range left {
		if w := visibleWidth(line); w > maxWidth {
			maxWidth = w
		}
	}

	rows := max(len(left), len(right))
	result := make([]string, rows)
	for i := 0; i < rows; i++ {
		var l, r string
		if i < len(left) {
			l = padMarkup(left[i], maxWidth)
		} else {
			l = strings.Repeat(" ", maxWidth)
		}
		if i < len(right) {
			r = right[i]
		}
		result[i] = l + gap + r
	}
	return strings.Join(result, "\n")
}

// stripMarkup remove tags de cor estilo tview.
func stripMarkup(text string) string {
	if text == "" {
		return ""
	}
	var b strings.Builder
	depth := 0
	for _, r := range text {
		if depth > 0 {
			if r == ']' {
				depth--
			}
			continue
		}
		if r == '[' {
			depth++
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// visibleWidth devolve a largura real de um texto com markup.
func visibleWidth(text string) int {
	stripped := stripMarkup(text)
	return runewidth.StringWidth(stripped)
}

// padMarkup completa um texto com espaços preservando largura visual.
func padMarkup(text string, width int) string {
	diff := width - visibleWidth(text)
	if diff <= 0 {
		return text
	}
	return text + strings.Repeat(" ", diff)
}
