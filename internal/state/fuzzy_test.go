package state

import "testing"

func TestLevenshteinDistance(t *testing.T) {
	tests := []struct {
		s1, s2 string
		want   int
	}{
		{"", "", 0},
		{"abc", "", 3},
		{"", "abc", 3},
		{"abc", "abc", 0},
		{"abc", "abd", 1},   // Substitution
		{"abc", "ab", 1},    // Deletion
		{"ab", "abc", 1},    // Insertion
		{"kitten", "sitting", 3},
		{"Saturday", "Sunday", 3},
		{"ServiceClass", "ServiceClas", 1},  // One char missing
		{"ServiceClass", "SeviceClass", 1},  // One char wrong
		{"note", "nots", 1},
	}

	for _, tc := range tests {
		got := levenshteinDistance(tc.s1, tc.s2)
		if got != tc.want {
			t.Errorf("levenshteinDistance(%q, %q) = %d; want %d", tc.s1, tc.s2, got, tc.want)
		}
	}
}

func TestFuzzyMatcher_Match(t *testing.T) {
	m := NewFuzzyMatcher()
	m.MaxDistance = 2

	tests := []struct {
		target    string
		candidate string
		wantScore MatchScore
	}{
		{"ServiceClass", "ServiceClass", MatchExact},
		{"serviceclass", "ServiceClass", MatchCaseInsensitive},
		{"Service", "ServiceClass", MatchPrefix},
		{"SrviceClass", "ServiceClass", MatchFuzzy},   // 1 char diff (not a prefix)
		{"ServiceClas", "ServiceClass", MatchPrefix},  // Prefix match (ServiceClas is prefix of ServiceClass)
		{"Completely", "Different", MatchNone},
		{"Note", "Notes", MatchPrefix},                // Prefix match
		{"Noet", "Notes", MatchFuzzy},                 // Typo - fuzzy match
	}

	for _, tc := range tests {
		score, _ := m.Match(tc.target, tc.candidate)
		if score != tc.wantScore {
			t.Errorf("Match(%q, %q) score = %d; want %d", tc.target, tc.candidate, score, tc.wantScore)
		}
	}
}

func TestFuzzyMatcher_FindBestMatches(t *testing.T) {
	m := NewFuzzyMatcher()
	m.MaxDistance = 2

	candidates := []MatchResult{
		{Path: "work/ServiceClass.md", PageID: "page1"},
		{Path: "work/ServiceClasses.md", PageID: "page2"},
		{Path: "work/Architecture.md", PageID: "page3"},
		{Path: "personal/Notes.md", PageID: "page4"},
	}

	tests := []struct {
		target    string
		wantFirst string
		wantCount int
	}{
		{"ServiceClass", "work/ServiceClass.md", 2},    // Exact + prefix
		{"SrviceClass", "work/ServiceClass.md", 1},     // Fuzzy match (only exact distance=1)
		{"Architecture", "work/Architecture.md", 1},
		{"Unknown", "", 0},
	}

	for _, tc := range tests {
		matches := m.FindBestMatches(tc.target, candidates, 5)

		if tc.wantCount == 0 {
			if len(matches) != 0 {
				t.Errorf("FindBestMatches(%q) got %d matches; want 0", tc.target, len(matches))
			}
			continue
		}

		if len(matches) < tc.wantCount {
			t.Errorf("FindBestMatches(%q) got %d matches; want at least %d", tc.target, len(matches), tc.wantCount)
			continue
		}

		if matches[0].Path != tc.wantFirst {
			t.Errorf("FindBestMatches(%q) first = %q; want %q", tc.target, matches[0].Path, tc.wantFirst)
		}
	}
}

func TestNormalizeForMatch(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"ServiceClass", "serviceclass"},
		{"Service Class", "service class"},
		{"service-class", "service class"},
		{"service_class", "service class"},
		{"Service.Class", "service class"},
		{"  multiple   spaces  ", "multiple spaces"},
	}

	for _, tc := range tests {
		got := normalizeForMatch(tc.input)
		if got != tc.want {
			t.Errorf("normalizeForMatch(%q) = %q; want %q", tc.input, got, tc.want)
		}
	}
}

func TestExtractName(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"work/notes/ServiceClass.md", "ServiceClass"},
		{"ServiceClass.md", "ServiceClass"},
		{"ServiceClass", "ServiceClass"},
		{"/full/path/to/Note.md", "Note"},
	}

	for _, tc := range tests {
		got := extractName(tc.path)
		if got != tc.want {
			t.Errorf("extractName(%q) = %q; want %q", tc.path, got, tc.want)
		}
	}
}
