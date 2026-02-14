package usercfg

import "testing"

func TestFuzzyMatch(t *testing.T) {
	tests := []struct {
		pattern  string
		target   string
		expected bool
	}{
		{"", "anything", true},
		{"", "", true},
		{"abc", "", false},
		{"abc", "abc", true},
		{"abc", "aabbcc", true},
		{"abc", "axbxcx", true},
		{"abc", "abcd", true},
		{"abc", "xyzabc", true},
		{"abc", "acb", false}, // Order matters
		{"bug", "fix login bug", true},
		{"fix", "fix login bug", true},
		{"login", "fix login bug", true},
		{"xyz", "fix login bug", false},
		{"CHANGE", "CHANGE-1234", true},
		{"ch1234", "CHANGE-1234", true},
		{"ch34", "CHANGE-1234", true},
		{"1234ch", "CHANGE-1234", false}, // Order matters
	}
	
	for _, test := range tests {
		result := FuzzyMatch(test.pattern, test.target)
		if result != test.expected {
			t.Errorf("FuzzyMatch(%q, %q) = %v, expected %v", test.pattern, test.target, result, test.expected)
		}
	}
}

func TestFuzzyScore(t *testing.T) {
	tests := []struct {
		pattern string
		target  string
		minScore int // minimum expected score, -1 for no match
	}{
		{"", "anything", 90}, // Empty pattern should score high
		{"abc", "nomatch", -1},
		{"bug", "bug", 90}, // Exact match should score high
		{"bug", "fix bug", 70}, // Good match
		{"bug", "fix login bug issue", 40}, // Longer text, lower score
		{"ch1234", "CHANGE-1234", 50}, // Decent match
	}
	
	for _, test := range tests {
		result := FuzzyScore(test.pattern, test.target)
		if test.minScore == -1 {
			if result != -1 {
				t.Errorf("FuzzyScore(%q, %q) = %d, expected -1 (no match)", test.pattern, test.target, result)
			}
		} else {
			if result < test.minScore {
				t.Errorf("FuzzyScore(%q, %q) = %d, expected >= %d", test.pattern, test.target, result, test.minScore)
			}
		}
	}
}

func TestNormalizeSearchText(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"", ""},
		{"abc", "abc"},
		{"ABC", "abc"},
		{"CHANGE-1234", "change-1234"},
		{"Fix: login bug", "fix login bug"},
		{"Feature/API-123", "featureapi-123"},
		{"Update (urgent)", "update urgent"},
	}
	
	for _, test := range tests {
		result := NormalizeSearchText(test.input)
		if result != test.expected {
			t.Errorf("NormalizeSearchText(%q) = %q, expected %q", test.input, result, test.expected)
		}
	}
}