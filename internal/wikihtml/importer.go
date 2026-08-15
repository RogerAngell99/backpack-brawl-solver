package wikihtml

import (
	"encoding/json"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

type Proposal struct {
	Item        ImportedItem     `json:"item"`
	Description string           `json:"description"`
	Recipes     []ImportedRecipe `json:"recipes"`
	TileGrid    [][]string       `json:"tile_grid"`
}

type ImportedItem struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Types       []string       `json:"types"`
	Shape       [][]int        `json:"shape"`
	Stars       []ImportedStar `json:"stars"`
	AbilityText string         `json:"ability_text"`
	SourceURL   string         `json:"source_url"`
	ImageURL    string         `json:"image_url"`
	ImagePath   string         `json:"image_path"`
	NeedsReview bool           `json:"needs_review"`
}

type ImportedStar struct {
	Offset            []int    `json:"offset"`
	TargetTypes       []string `json:"target_types"`
	TargetItems       []string `json:"target_items"`
	ExcludeSourceItem bool     `json:"exclude_source_item,omitempty"`
	EffectText        string   `json:"effect_text"`
}

type ImportedRecipe struct {
	Result      string   `json:"result"`
	Anchor      string   `json:"anchor"`
	Ingredients []string `json:"ingredients"`
	SourceURL   string   `json:"source_url"`
}

func ExtractFile(path string) (Proposal, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return Proposal{}, err
	}
	return Extract(string(content))
}

func ExtractPaths(paths []string) ([]Proposal, error) {
	files, err := expandPaths(paths)
	if err != nil {
		return nil, err
	}

	proposals := make([]Proposal, 0, len(files))
	for _, file := range files {
		proposal, err := ExtractFile(file)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", file, err)
		}
		proposals = append(proposals, proposal)
	}
	return proposals, nil
}

func Extract(content string) (Proposal, error) {
	name := extractName(content)
	if name == "" {
		return Proposal{}, fmt.Errorf("could not find item name")
	}

	shape, stars, tileGrid, err := extractTiles(content)
	if err != nil {
		return Proposal{}, err
	}

	sourceURL := extractCanonicalURL(content)
	itemID := slugify(name)
	recipes := extractOwnRecipes(content, name, sourceURL)
	description := extractMetaDescription(content)

	return Proposal{
		Item: ImportedItem{
			ID:          itemID,
			Name:        name,
			Types:       extractTypes(content),
			Shape:       shape,
			Stars:       stars,
			AbilityText: extractAbilityText(content),
			SourceURL:   sourceURL,
			ImageURL:    extractImageURL(content),
			ImagePath:   "assets/items/" + itemID + ".png",
			NeedsReview: true,
		},
		Description: description,
		Recipes:     recipes,
		TileGrid:    tileGrid,
	}, nil
}

func MarshalProposal(proposal Proposal) ([]byte, error) {
	return json.MarshalIndent(proposal, "", "  ")
}

func MarshalProposals(proposals []Proposal) ([]byte, error) {
	return json.MarshalIndent(proposals, "", "  ")
}

func ReviewText(proposals []Proposal) string {
	var builder strings.Builder
	for idx, proposal := range proposals {
		if idx > 0 {
			builder.WriteString("\n")
		}
		writeProposalReview(&builder, proposal)
	}
	return strings.TrimRight(builder.String(), "\n")
}

func writeProposalReview(builder *strings.Builder, proposal Proposal) {
	fmt.Fprintf(builder, "=== %s (%s) ===\n", proposal.Item.Name, proposal.Item.ID)
	fmt.Fprintf(builder, "Types: %s\n", joinOrNone(proposal.Item.Types, ", "))
	fmt.Fprintf(builder, "Image URL: %s\n", valueOrNone(proposal.Item.ImageURL))
	fmt.Fprintf(builder, "Image path: %s\n", valueOrNone(proposal.Item.ImagePath))
	fmt.Fprintf(builder, "Shape: %s\n", shapeSummary(proposal.Item.Shape))
	fmt.Fprintf(builder, "Stars: %d %s\n", len(proposal.Item.Stars), formatStarOffsets(proposal.Item.Stars))
	builder.WriteString("Grid (#=item, *=star, .=empty):\n")
	builder.WriteString(reviewGrid(proposal.Item.Shape, proposal.Item.Stars))
	builder.WriteString("\n")
	if len(proposal.Recipes) == 0 {
		builder.WriteString("Recipe: none\n")
	} else {
		builder.WriteString("Recipe:\n")
		for _, recipe := range proposal.Recipes {
			fmt.Fprintf(builder, "  %s = %s (anchor: %s)\n", recipe.Result, strings.Join(recipe.Ingredients, " + "), recipe.Anchor)
		}
	}
	if proposal.Item.AbilityText != "" {
		fmt.Fprintf(builder, "Ability preview: %s\n", compactPreview(proposal.Item.AbilityText, 220))
	}
}

func joinOrNone(values []string, sep string) string {
	if len(values) == 0 {
		return "none"
	}
	return strings.Join(values, sep)
}

func valueOrNone(value string) string {
	if value == "" {
		return "none"
	}
	return value
}

func shapeSummary(shape [][]int) string {
	if len(shape) == 0 {
		return "0 cells"
	}
	maxRow := shape[0][0]
	maxCol := shape[0][1]
	for _, cell := range shape {
		if len(cell) != 2 {
			continue
		}
		if cell[0] > maxRow {
			maxRow = cell[0]
		}
		if cell[1] > maxCol {
			maxCol = cell[1]
		}
	}
	cellWord := "cells"
	if len(shape) == 1 {
		cellWord = "cell"
	}
	return fmt.Sprintf("%d %s x %d %s box, %d %s", maxRow+1, pluralWord(maxRow+1, "row", "rows"), maxCol+1, pluralWord(maxCol+1, "col", "cols"), len(shape), cellWord)
}

func pluralWord(count int, singular string, plural string) string {
	if count == 1 {
		return singular
	}
	return plural
}

func formatStarOffsets(stars []ImportedStar) string {
	if len(stars) == 0 {
		return "[]"
	}
	parts := make([]string, 0, len(stars))
	for _, star := range stars {
		if len(star.Offset) != 2 {
			continue
		}
		parts = append(parts, "["+strconv.Itoa(star.Offset[0])+","+strconv.Itoa(star.Offset[1])+"]")
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

func reviewGrid(shape [][]int, stars []ImportedStar) string {
	minRow, minCol, maxRow, maxCol := reviewBounds(shape, stars)
	rows := maxRow - minRow + 1
	cols := maxCol - minCol + 1
	cells := make([][]string, rows)
	for row := range cells {
		cells[row] = make([]string, cols)
		for col := range cells[row] {
			cells[row][col] = "."
		}
	}
	for _, star := range stars {
		if len(star.Offset) != 2 {
			continue
		}
		cells[star.Offset[0]-minRow][star.Offset[1]-minCol] = "*"
	}
	for _, cell := range shape {
		if len(cell) != 2 {
			continue
		}
		cells[cell[0]-minRow][cell[1]-minCol] = "#"
	}

	lines := make([]string, 0, rows)
	for _, row := range cells {
		lines = append(lines, strings.Join(row, " "))
	}
	return strings.Join(lines, "\n")
}

func reviewBounds(shape [][]int, stars []ImportedStar) (int, int, int, int) {
	minRow, minCol, maxRow, maxCol := 0, 0, 0, 0
	initialized := false
	visit := func(row int, col int) {
		if !initialized {
			minRow, minCol, maxRow, maxCol = row, col, row, col
			initialized = true
			return
		}
		if row < minRow {
			minRow = row
		}
		if col < minCol {
			minCol = col
		}
		if row > maxRow {
			maxRow = row
		}
		if col > maxCol {
			maxCol = col
		}
	}
	for _, cell := range shape {
		if len(cell) == 2 {
			visit(cell[0], cell[1])
		}
	}
	for _, star := range stars {
		if len(star.Offset) == 2 {
			visit(star.Offset[0], star.Offset[1])
		}
	}
	return minRow, minCol, maxRow, maxCol
}

func compactPreview(value string, limit int) string {
	value = strings.Join(strings.Fields(value), " ")
	if len(value) <= limit {
		return value
	}
	if limit <= 3 {
		return value[:limit]
	}
	return strings.TrimSpace(value[:limit-3]) + "..."
}

func expandPaths(paths []string) ([]string, error) {
	seen := map[string]bool{}
	var files []string
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			return nil, err
		}
		if info.IsDir() {
			matches, err := filepath.Glob(filepath.Join(path, "*Backpack Brawl Wiki*.html"))
			if err != nil {
				return nil, err
			}
			if len(matches) == 0 {
				matches, err = filepath.Glob(filepath.Join(path, "*.html"))
				if err != nil {
					return nil, err
				}
			}
			sort.Strings(matches)
			for _, match := range matches {
				if !seen[match] {
					files = append(files, match)
					seen[match] = true
				}
			}
			continue
		}
		if !seen[path] {
			files = append(files, path)
			seen[path] = true
		}
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no HTML files found")
	}
	return files, nil
}

func extractName(content string) string {
	if match := regexp.MustCompile(`(?is)<div class="druid-title">\s*([^<]+?)\s*</div>`).FindStringSubmatch(content); len(match) == 2 {
		return cleanText(match[1])
	}
	if match := regexp.MustCompile(`(?is)<meta property="og:title" content="([^"]+)"`).FindStringSubmatch(content); len(match) == 2 {
		return cleanText(match[1])
	}
	if match := regexp.MustCompile(`(?is)<title>([^<]+?) - Backpack Brawl Wiki</title>`).FindStringSubmatch(content); len(match) == 2 {
		return cleanText(match[1])
	}
	return ""
}

func extractCanonicalURL(content string) string {
	if match := regexp.MustCompile(`(?is)<link rel="canonical" href="([^"]+)"`).FindStringSubmatch(content); len(match) == 2 {
		return html.UnescapeString(match[1])
	}
	return ""
}

func extractImageURL(content string) string {
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`(?is)<meta property="og:image" content="([^"]+)"`),
		regexp.MustCompile(`(?is)<div class="druid-main-image".*?<img[^>]+src="([^"]+)"`),
	}
	for _, pattern := range patterns {
		if match := pattern.FindStringSubmatch(content); len(match) == 2 {
			return absoluteWikiURL(html.UnescapeString(match[1]))
		}
	}
	return ""
}

func absoluteWikiURL(value string) string {
	if strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://") {
		return value
	}
	if strings.HasPrefix(value, "//") {
		return "https:" + value
	}
	if strings.HasPrefix(value, "/") {
		return "https://backpackbrawl.wiki.gg" + value
	}
	return value
}

func extractTypes(content string) []string {
	block := extractBetween(content, `druid-row druid-row-Type`, `druid-row druid-row-Cost`)
	if block == "" {
		return nil
	}

	seen := map[string]bool{}
	var types []string
	for _, match := range regexp.MustCompile(`title="Category:([^"]+?)(?:_Type_Items| Type Items)"`).FindAllStringSubmatch(block, -1) {
		itemType := strings.ReplaceAll(html.UnescapeString(match[1]), "_", " ")
		if itemType != "" && !seen[itemType] {
			seen[itemType] = true
			types = append(types, itemType)
		}
	}
	if len(types) > 0 {
		return types
	}

	text := cleanText(stripTags(block))
	text = strings.TrimPrefix(text, "Type")
	fields := strings.Fields(text)
	for _, field := range fields {
		if !seen[field] {
			seen[field] = true
			types = append(types, field)
		}
	}
	return types
}

func extractTiles(content string) ([][]int, []ImportedStar, [][]string, error) {
	block := extractBetween(content, `druid-row druid-row-Grid`, `</table>`)
	if block == "" {
		return nil, nil, nil, fmt.Errorf("could not find Tiles grid")
	}

	rowMatches := regexp.MustCompile(`(?is)<tr[^>]*>(.*?)</tr>`).FindAllStringSubmatch(block, -1)
	if len(rowMatches) == 0 {
		return nil, nil, nil, fmt.Errorf("Tiles grid has no rows")
	}

	var itemCells [][2]int
	var starCells [][2]int
	var tileGrid [][]string
	for rowIdx, rowMatch := range rowMatches {
		tileMatches := regexp.MustCompile(`alt="(Empty Tile|Item Tile|Star Tile|Star)"`).FindAllStringSubmatch(rowMatch[1], -1)
		if len(tileMatches) == 0 {
			continue
		}
		var row []string
		for colIdx, tileMatch := range tileMatches {
			tile := strings.TrimSuffix(tileMatch[1], " Tile")
			row = append(row, tile)
			switch tile {
			case "Item":
				itemCells = append(itemCells, [2]int{rowIdx, colIdx})
			case "Star":
				starCells = append(starCells, [2]int{rowIdx, colIdx})
			}
		}
		tileGrid = append(tileGrid, row)
	}
	if len(itemCells) == 0 {
		return nil, nil, tileGrid, fmt.Errorf("Tiles grid has no Item tiles")
	}

	minRow := itemCells[0][0]
	minCol := itemCells[0][1]
	for _, cell := range itemCells {
		if cell[0] < minRow {
			minRow = cell[0]
		}
		if cell[1] < minCol {
			minCol = cell[1]
		}
	}

	shape := make([][]int, 0, len(itemCells))
	for _, cell := range itemCells {
		shape = append(shape, []int{cell[0] - minRow, cell[1] - minCol})
	}
	sort.Slice(shape, func(i, j int) bool {
		if shape[i][0] != shape[j][0] {
			return shape[i][0] < shape[j][0]
		}
		return shape[i][1] < shape[j][1]
	})

	stars := make([]ImportedStar, 0, len(starCells))
	for _, cell := range starCells {
		stars = append(stars, ImportedStar{
			Offset:      []int{cell[0] - minRow, cell[1] - minCol},
			TargetTypes: []string{},
			TargetItems: []string{},
			EffectText:  "",
		})
	}
	sort.Slice(stars, func(i, j int) bool {
		if stars[i].Offset[0] != stars[j].Offset[0] {
			return stars[i].Offset[0] < stars[j].Offset[0]
		}
		return stars[i].Offset[1] < stars[j].Offset[1]
	})

	return shape, stars, tileGrid, nil
}

func extractMetaDescription(content string) string {
	if match := regexp.MustCompile(`(?is)<meta name="description" content="([^"]*)"`).FindStringSubmatch(content); len(match) == 2 {
		return cleanText(strings.ReplaceAll(html.UnescapeString(match[1]), "\n", " "))
	}
	return ""
}

func extractAbilityText(content string) string {
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`(?is)<h2[^>]*>\s*<span[^>]*id="Initial_Abilities"[^>]*>.*?</h2>(.*?)(?:<h2|$)`),
		regexp.MustCompile(`(?is)<span[^>]*id="Initial_Abilities"[^>]*>.*?</span>\s*</h2>(.*?)(?:<h2|$)`),
	}
	for _, pattern := range patterns {
		if match := pattern.FindStringSubmatch(content); len(match) == 2 {
			return cleanHTMLText(match[1])
		}
	}
	return ""
}

func extractOwnRecipes(content string, itemName string, sourceURL string) []ImportedRecipe {
	table := extractBetween(content, `id="Recipes"`, `</table>`)
	if table == "" {
		return nil
	}

	var recipes []ImportedRecipe
	currentResult := ""
	currentIngredients := []string{}
	flush := func() {
		if currentResult == itemName && len(currentIngredients) > 0 {
			ingredientIDs := make([]string, 0, len(currentIngredients))
			for _, ingredient := range currentIngredients {
				ingredientIDs = append(ingredientIDs, slugify(ingredient))
			}
			recipes = append(recipes, ImportedRecipe{
				Result:      slugify(currentResult),
				Anchor:      ingredientIDs[0],
				Ingredients: ingredientIDs,
				SourceURL:   sourceURL,
			})
		}
	}

	rowMatches := regexp.MustCompile(`(?is)<tr[^>]*>(.*?)</tr>`).FindAllStringSubmatch(table, -1)
	for _, rowMatch := range rowMatches {
		row := rowMatch[1]
		cells := regexp.MustCompile(`(?is)<td[^>]*>(.*?)</td>`).FindAllStringSubmatch(row, -1)
		if len(cells) == 0 {
			continue
		}

		if strings.Contains(row, "rowspan") && strings.Contains(row, "<b>") {
			flush()
			currentResult = extractBoldText(row)
			currentIngredients = nil
		}

		ingredient := extractIngredientName(cells[len(cells)-1][1])
		if ingredient != "" {
			currentIngredients = append(currentIngredients, ingredient)
		}
	}
	flush()

	return recipes
}

func extractBoldText(row string) string {
	if match := regexp.MustCompile(`(?is)<b>(.*?)</b>`).FindStringSubmatch(row); len(match) == 2 {
		return cleanHTMLText(match[1])
	}
	return ""
}

func extractIngredientName(cell string) string {
	withoutImages := regexp.MustCompile(`(?is)<img[^>]*>`).ReplaceAllString(cell, "")
	text := cleanHTMLText(withoutImages)
	if text == "" || text == "Image" || text == "Recipe" || text == "Item" {
		return ""
	}
	return text
}

func extractBetween(content string, startNeedle string, endNeedle string) string {
	start := strings.Index(content, startNeedle)
	if start < 0 {
		return ""
	}
	end := strings.Index(content[start:], endNeedle)
	if end < 0 {
		return content[start:]
	}
	return content[start : start+end+len(endNeedle)]
}

func cleanHTMLText(value string) string {
	withImageAlt := regexp.MustCompile(`(?is)<img[^>]*alt="([^"]*)"[^>]*>`).ReplaceAllString(value, " $1 ")
	withBreaks := regexp.MustCompile(`(?i)<br\s*/?>`).ReplaceAllString(withImageAlt, "\n")
	withParagraphs := regexp.MustCompile(`(?i)</(p|li|ul|h2|b|td|tr)>`).ReplaceAllString(withBreaks, "\n")
	return cleanText(stripTags(withParagraphs))
}

func stripTags(value string) string {
	return regexp.MustCompile(`(?is)<[^>]+>`).ReplaceAllString(value, " ")
}

func cleanText(value string) string {
	value = html.UnescapeString(value)
	value = strings.ReplaceAll(value, "\r", "\n")
	lines := strings.Split(value, "\n")
	var cleanedLines []string
	for _, line := range lines {
		cleaned := strings.Join(strings.Fields(line), " ")
		if cleaned != "" {
			cleanedLines = append(cleanedLines, cleaned)
		}
	}
	return strings.Join(cleanedLines, "\n")
}

func slugify(value string) string {
	var builder strings.Builder
	lastUnderscore := false
	for _, r := range strings.ToLower(value) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			builder.WriteRune(r)
			lastUnderscore = false
			continue
		}
		if !lastUnderscore {
			builder.WriteByte('_')
			lastUnderscore = true
		}
	}
	return strings.Trim(builder.String(), "_")
}
