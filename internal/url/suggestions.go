package url

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// SuggestionEngine generates smart short code suggestions
type SuggestionEngine struct {
	generator *ShortCodeGenerator
}

// NewSuggestionEngine creates a new suggestion engine
func NewSuggestionEngine(generator *ShortCodeGenerator) *SuggestionEngine {
	return &SuggestionEngine{
		generator: generator,
	}
}

// GenerateSmartSuggestions generates intelligent short code suggestions
func (e *SuggestionEngine) GenerateSmartSuggestions(ctx context.Context, originalURL string, count int) ([]string, error) {
	if count <= 0 {
		count = 5
	}
	if count > 10 {
		count = 10
	}

	var suggestions []string

	// Parse URL
	parsedURL, err := url.Parse(originalURL)
	if err != nil {
		// If URL parsing fails, fall back to random generation
		return e.generator.GenerateSuggestions(ctx, originalURL, count)
	}

	// Strategy 1: Domain-based suggestions
	domainSuggestions := e.generateDomainSuggestions(parsedURL, count/3)
	suggestions = append(suggestions, domainSuggestions...)

	// Strategy 2: Path-based suggestions
	pathSuggestions := e.generatePathSuggestions(parsedURL, count/3)
	suggestions = append(suggestions, pathSuggestions...)

	// Strategy 3: Content-based suggestions
	contentSuggestions := e.generateContentSuggestions(parsedURL, count/3)
	suggestions = append(suggestions, contentSuggestions...)

	// Remove duplicates and validate
	suggestions = e.deduplicateAndValidate(ctx, suggestions)

	// Fill remaining slots with random codes if needed
	for len(suggestions) < count {
		randomCode, _, err := e.generator.generateRandomCode(ctx, originalURL)
		if err != nil {
			break
		}
		suggestions = append(suggestions, randomCode)
	}

	// Return only the requested number
	if len(suggestions) > count {
		suggestions = suggestions[:count]
	}

	return suggestions, nil
}

// generateDomainSuggestions generates suggestions based on domain name
func (e *SuggestionEngine) generateDomainSuggestions(parsedURL *url.URL, count int) []string {
	var suggestions []string
	domain := parsedURL.Host

	// Remove www prefix
	if strings.HasPrefix(domain, "www.") {
		domain = domain[4:]
	}

	// Split domain into parts
	parts := strings.Split(domain, ".")
	if len(parts) == 0 {
		return suggestions
	}

	mainDomain := parts[0]

	// Strategy 1: First few characters of domain
	if len(mainDomain) >= 3 {
		for i := 3; i <= len(mainDomain) && i <= e.generator.config.MaxRandomLength; i++ {
			suggestions = append(suggestions, mainDomain[:i])
			if len(suggestions) >= count {
				break
			}
		}
	}

	// Strategy 2: Remove vowels for abbreviation
	if len(suggestions) < count {
		abbreviated := e.removeVowels(mainDomain)
		if len(abbreviated) >= e.generator.config.MinRandomLength && len(abbreviated) <= e.generator.config.MaxRandomLength {
			suggestions = append(suggestions, abbreviated)
		}
	}

	// Strategy 3: Take consonant clusters
	if len(suggestions) < count {
		consonantClusters := e.extractConsonantClusters(mainDomain)
		for _, cluster := range consonantClusters {
			if len(cluster) >= e.generator.config.MinRandomLength && len(cluster) <= e.generator.config.MaxRandomLength {
				suggestions = append(suggestions, cluster)
				if len(suggestions) >= count {
					break
				}
			}
		}
	}

	return e.limitSuggestions(suggestions, count)
}

// generatePathSuggestions generates suggestions based on URL path
func (e *SuggestionEngine) generatePathSuggestions(parsedURL *url.URL, count int) []string {
	var suggestions []string
	path := strings.Trim(parsedURL.Path, "/")

	if path == "" {
		return suggestions
	}

	// Split path into segments
	segments := strings.Split(path, "/")

	for _, segment := range segments {
		if segment == "" {
			continue
		}

		// Clean segment
		cleanSegment := e.cleanPathSegment(segment)
		if len(cleanSegment) < e.generator.config.MinRandomLength {
			continue
		}

		// Add full segment if it fits
		if len(cleanSegment) <= e.generator.config.MaxRandomLength {
			suggestions = append(suggestions, cleanSegment)
		}

		// Add truncated version
		if len(cleanSegment) > e.generator.config.MaxRandomLength {
			suggestions = append(suggestions, cleanSegment[:e.generator.config.MaxRandomLength])
		}

		// Add abbreviated version
		abbreviated := e.abbreviateSegment(cleanSegment)
		if abbreviated != "" {
			suggestions = append(suggestions, abbreviated)
		}

		if len(suggestions) >= count*2 {
			break
		}
	}

	return e.limitSuggestions(suggestions, count)
}

// generateContentSuggestions generates suggestions based on URL parameters and fragments
func (e *SuggestionEngine) generateContentSuggestions(parsedURL *url.URL, count int) []string {
	var suggestions []string

	// Extract meaningful parameters
	params := parsedURL.Query()
	meaningfulParams := []string{"id", "name", "title", "slug", "page", "article", "post", "product"}

	for _, param := range meaningfulParams {
		if value := params.Get(param); value != "" {
			cleanValue := e.cleanParameterValue(value)
			if len(cleanValue) >= e.generator.config.MinRandomLength && len(cleanValue) <= e.generator.config.MaxRandomLength {
				suggestions = append(suggestions, cleanValue)
				if len(suggestions) >= count {
					break
				}
			}
		}
	}

	// Extract from fragment if present
	if parsedURL.Fragment != "" {
		cleanFragment := e.cleanFragment(parsedURL.Fragment)
		if len(cleanFragment) >= e.generator.config.MinRandomLength && len(cleanFragment) <= e.generator.config.MaxRandomLength {
			suggestions = append(suggestions, cleanFragment)
		}
	}

	return e.limitSuggestions(suggestions, count)
}

// cleanPathSegment cleans a path segment for use as short code
func (e *SuggestionEngine) cleanPathSegment(segment string) string {
	// Remove file extensions
	if dotIndex := strings.LastIndex(segment, "."); dotIndex > 0 {
		segment = segment[:dotIndex]
	}

	// Remove non-alphanumeric characters
	cleaned := regexp.MustCompile(`[^a-zA-Z0-9]`).ReplaceAllString(segment, "")

	// Convert to lowercase
	return strings.ToLower(cleaned)
}

// cleanParameterValue cleans a URL parameter value
func (e *SuggestionEngine) cleanParameterValue(value string) string {
	// Remove non-alphanumeric characters
	cleaned := regexp.MustCompile(`[^a-zA-Z0-9]`).ReplaceAllString(value, "")

	// Convert to lowercase
	cleaned = strings.ToLower(cleaned)

	// Truncate if too long
	if len(cleaned) > e.generator.config.MaxRandomLength {
		cleaned = cleaned[:e.generator.config.MaxRandomLength]
	}

	return cleaned
}

// cleanFragment cleans a URL fragment
func (e *SuggestionEngine) cleanFragment(fragment string) string {
	// Remove non-alphanumeric characters
	cleaned := regexp.MustCompile(`[^a-zA-Z0-9]`).ReplaceAllString(fragment, "")

	// Convert to lowercase
	cleaned = strings.ToLower(cleaned)

	// Truncate if too long
	if len(cleaned) > e.generator.config.MaxRandomLength {
		cleaned = cleaned[:e.generator.config.MaxRandomLength]
	}

	return cleaned
}

// removeVowels removes vowels from a string for abbreviation
func (e *SuggestionEngine) removeVowels(s string) string {
	vowels := "aeiouAEIOU"
	var result strings.Builder

	for i, char := range s {
		// Always keep the first character
		if i == 0 {
			result.WriteRune(char)
		} else if !strings.ContainsRune(vowels, char) {
			result.WriteRune(char)
		}
	}

	return strings.ToLower(result.String())
}

// extractConsonantClusters extracts consonant clusters from a string
func (e *SuggestionEngine) extractConsonantClusters(s string) []string {
	vowels := "aeiouAEIOU"
	var clusters []string
	var currentCluster strings.Builder

	for _, char := range s {
		if strings.ContainsRune(vowels, char) {
			if currentCluster.Len() > 0 {
				cluster := strings.ToLower(currentCluster.String())
				if len(cluster) >= 2 {
					clusters = append(clusters, cluster)
				}
				currentCluster.Reset()
			}
		} else {
			currentCluster.WriteRune(char)
		}
	}

	// Add final cluster if exists
	if currentCluster.Len() > 0 {
		cluster := strings.ToLower(currentCluster.String())
		if len(cluster) >= 2 {
			clusters = append(clusters, cluster)
		}
	}

	return clusters
}

// abbreviateSegment creates an abbreviation of a path segment
func (e *SuggestionEngine) abbreviateSegment(segment string) string {
	if len(segment) <= 4 {
		return segment
	}

	// Take first character + every nth character
	var result strings.Builder
	result.WriteRune(rune(segment[0]))

	step := len(segment) / (e.generator.config.MaxRandomLength - 1)
	if step < 1 {
		step = 1
	}

	for i := step; i < len(segment) && result.Len() < e.generator.config.MaxRandomLength; i += step {
		result.WriteRune(rune(segment[i]))
	}

	abbreviated := strings.ToLower(result.String())
	if len(abbreviated) < e.generator.config.MinRandomLength {
		return ""
	}

	return abbreviated
}

// deduplicateAndValidate removes duplicates and validates suggestions
func (e *SuggestionEngine) deduplicateAndValidate(ctx context.Context, suggestions []string) []string {
	seen := make(map[string]bool)
	var validated []string

	for _, suggestion := range suggestions {
		if seen[suggestion] {
			continue
		}

		// Validate the suggestion
		if err := e.generator.validateCustomCode(suggestion); err == nil {
			// Check if available
			if exists, err := e.generator.codeExists(ctx, suggestion); err == nil && !exists {
				seen[suggestion] = true
				validated = append(validated, suggestion)
			}
		}
	}

	return validated
}

// limitSuggestions limits the number of suggestions
func (e *SuggestionEngine) limitSuggestions(suggestions []string, count int) []string {
	if len(suggestions) <= count {
		return suggestions
	}
	return suggestions[:count]
}

// GenerateThematicSuggestions generates suggestions based on themes or categories
func (e *SuggestionEngine) GenerateThematicSuggestions(ctx context.Context, theme string, count int) ([]string, error) {
	themes := map[string][]string{
		"tech": {"api", "dev", "app", "web", "bot", "sys", "net", "cpu", "ram", "ssd"},
		"social": {"msg", "chat", "post", "like", "tag", "user", "team", "group", "friend", "follow"},
		"business": {"biz", "corp", "inc", "ltd", "pro", "exec", "mgr", "ceo", "cfo", "dept"},
		"creative": {"art", "pic", "img", "vid", "music", "draw", "paint", "sketch", "design", "color"},
		"gaming": {"play", "game", "win", "lose", "level", "boss", "quest", "loot", "guild", "raid"},
		"sports": {"goal", "score", "team", "match", "league", "cup", "win", "champ", "race", "run"},
		"food": {"eat", "cook", "food", "meal", "dish", "taste", "spice", "sweet", "salt", "fresh"},
		"travel": {"trip", "fly", "drive", "visit", "tour", "hotel", "beach", "city", "country", "map"},
	}

	themeWords, exists := themes[strings.ToLower(theme)]
	if !exists {
		// Fall back to random generation
		return e.generator.GenerateSuggestions(ctx, "", count)
	}

	var suggestions []string
	for _, word := range themeWords {
		if len(word) >= e.generator.config.MinRandomLength && len(word) <= e.generator.config.MaxRandomLength {
			if exists, err := e.generator.codeExists(ctx, word); err == nil && !exists {
				suggestions = append(suggestions, word)
				if len(suggestions) >= count {
					break
				}
			}
		}
	}

	// Fill remaining with variations
	for len(suggestions) < count && len(themeWords) > 0 {
		for _, word := range themeWords {
			if len(suggestions) >= count {
				break
			}

			// Add numbers to create variations
			for i := 1; i <= 99; i++ {
				variation := word + fmt.Sprintf("%d", i)
				if len(variation) <= e.generator.config.MaxRandomLength {
					if exists, err := e.generator.codeExists(ctx, variation); err == nil && !exists {
						suggestions = append(suggestions, variation)
						break
					}
				}
			}
		}
		break // Prevent infinite loop
	}

	return suggestions, nil
}