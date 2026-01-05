package parser

import (
	"regexp"
	"strings"
)

var (
	// dataviewBlockRegex matches ```dataview ... ``` code blocks.
	dataviewBlockRegex = regexp.MustCompile("(?s)```dataview\n(.+?)\n```")

	// dataviewInlineRegex matches inline dataview queries like `=this.file.name`.
	dataviewInlineRegex = regexp.MustCompile("`=([^`]+)`")

	// dataviewJSRegex matches ```dataviewjs ... ``` code blocks.
	dataviewJSRegex = regexp.MustCompile("(?s)```dataviewjs\n(.+?)\n```")
)

// DataviewQueryType represents the type of dataview query.
type DataviewQueryType string

const (
	// QueryTypeTable is a TABLE query.
	QueryTypeTable DataviewQueryType = "TABLE"
	// QueryTypeList is a LIST query.
	QueryTypeList DataviewQueryType = "LIST"
	// QueryTypeTask is a TASK query.
	QueryTypeTask DataviewQueryType = "TASK"
	// QueryTypeCalendar is a CALENDAR query.
	QueryTypeCalendar DataviewQueryType = "CALENDAR"
	// QueryTypeInline is an inline query like `=this.file.name`.
	QueryTypeInline DataviewQueryType = "INLINE"
	// QueryTypeJS is a dataviewjs query.
	QueryTypeJS DataviewQueryType = "JS"
)

// DataviewQuery represents a dataview query found in content.
type DataviewQuery struct {
	// Raw is the original query text.
	Raw string

	// Type is the query type (TABLE, LIST, TASK, CALENDAR, INLINE, JS).
	Type DataviewQueryType

	// StartPos is the byte offset where the query starts.
	StartPos int

	// EndPos is the byte offset where the query ends.
	EndPos int

	// IsInline indicates if this is an inline query.
	IsInline bool
}

// ExtractDataviewQueries finds all dataview blocks and inline queries in content.
func ExtractDataviewQueries(content []byte) []DataviewQuery {
	var queries []DataviewQuery

	// Find block queries (```dataview ... ```).
	matches := dataviewBlockRegex.FindAllSubmatchIndex(content, -1)
	for _, match := range matches {
		if len(match) >= 4 {
			query := string(content[match[2]:match[3]])
			queries = append(queries, DataviewQuery{
				Raw:      query,
				Type:     parseQueryType(query),
				StartPos: match[0],
				EndPos:   match[1],
				IsInline: false,
			})
		}
	}

	// Find dataviewjs blocks.
	jsMatches := dataviewJSRegex.FindAllSubmatchIndex(content, -1)
	for _, match := range jsMatches {
		if len(match) >= 4 {
			query := string(content[match[2]:match[3]])
			queries = append(queries, DataviewQuery{
				Raw:      query,
				Type:     QueryTypeJS,
				StartPos: match[0],
				EndPos:   match[1],
				IsInline: false,
			})
		}
	}

	// Find inline queries (`=...`).
	inlineMatches := dataviewInlineRegex.FindAllSubmatchIndex(content, -1)
	for _, match := range inlineMatches {
		if len(match) >= 4 {
			query := string(content[match[2]:match[3]])
			queries = append(queries, DataviewQuery{
				Raw:      query,
				Type:     QueryTypeInline,
				StartPos: match[0],
				EndPos:   match[1],
				IsInline: true,
			})
		}
	}

	return queries
}

// parseQueryType determines the query type from the query text.
func parseQueryType(query string) DataviewQueryType {
	// Normalize and check first word.
	upper := strings.ToUpper(strings.TrimSpace(query))

	switch {
	case strings.HasPrefix(upper, "TABLE"):
		return QueryTypeTable
	case strings.HasPrefix(upper, "LIST"):
		return QueryTypeList
	case strings.HasPrefix(upper, "TASK"):
		return QueryTypeTask
	case strings.HasPrefix(upper, "CALENDAR"):
		return QueryTypeCalendar
	default:
		// Default to TABLE if type is unclear.
		return QueryTypeTable
	}
}

// SnapshotPlaceholder returns a placeholder block for a dataview query.
// This is used when dataview execution is not available.
func SnapshotPlaceholder(query DataviewQuery) string {
	if query.IsInline {
		return "[dataview: " + query.Raw + "]"
	}

	return "> [!info] Dataview Query\n" +
		"> This query requires Obsidian to execute:\n" +
		"> ```\n" +
		"> " + strings.ReplaceAll(query.Raw, "\n", "\n> ") + "\n" +
		"> ```\n"
}

// ContainsDataview checks if content contains any dataview queries.
func ContainsDataview(content []byte) bool {
	return dataviewBlockRegex.Match(content) ||
		dataviewInlineRegex.Match(content) ||
		dataviewJSRegex.Match(content)
}
