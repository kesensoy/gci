package usercfg

import (
	"strings"
	"unicode"
)

// FuzzyMatch implements a simple fuzzy string matching algorithm
// Returns true if all characters in pattern appear in target in order (case-insensitive)
func FuzzyMatch(pattern, target string) bool {
	if pattern == "" {
		return true
	}
	if target == "" {
		return false
	}
	
	pattern = strings.ToLower(pattern)
	target = strings.ToLower(target)
	
	patternIdx := 0
	targetIdx := 0
	
	for patternIdx < len(pattern) && targetIdx < len(target) {
		if pattern[patternIdx] == target[targetIdx] {
			patternIdx++
		}
		targetIdx++
	}
	
	return patternIdx == len(pattern)
}

// FuzzyScore calculates a fuzzy match score (higher is better)
// Returns -1 if no match, 0-100 for match quality
func FuzzyScore(pattern, target string) int {
	if !FuzzyMatch(pattern, target) {
		return -1
	}
	
	if pattern == "" {
		return 100
	}
	
	pattern = strings.ToLower(pattern)
	target = strings.ToLower(target)
	
	// Simple scoring: favor consecutive matches and shorter targets
	score := 0
	patternIdx := 0
	consecutiveMatches := 0
	
	for i, char := range target {
		if patternIdx < len(pattern) && rune(pattern[patternIdx]) == char {
			patternIdx++
			consecutiveMatches++
			score += 10 + consecutiveMatches // Bonus for consecutive matches
		} else {
			consecutiveMatches = 0
		}
		
		// Penalty for length (prefer shorter matches)
		if i > len(pattern)*3 {
			score -= 1
		}
	}
	
	// Bonus for exact matches
	if strings.Contains(target, pattern) {
		score += 20
	}
	
	// Normalize to 0-100 range
	maxScore := len(pattern) * 15
	if score > maxScore {
		score = maxScore
	}
	
	return (score * 100) / maxScore
}

// NormalizeSearchText normalizes text for searching by removing common punctuation
// and converting to lowercase
func NormalizeSearchText(text string) string {
	var result strings.Builder
	result.Grow(len(text))
	
	for _, r := range strings.ToLower(text) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == ' ' || r == '-' {
			result.WriteRune(r)
		}
	}
	
	return result.String()
}