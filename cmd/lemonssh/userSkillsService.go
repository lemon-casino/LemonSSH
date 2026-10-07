package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"
)

const (
	userSkillsDirName             = "Skills"
	userSkillsReadmeName          = "README.txt"
	maxUserSkillBytes             = 24 * 1024
	maxUserSkillDescription       = 500
	maxUserSkillIndex             = 8
	maxUserSkillIndexDescription  = 160
	maxUserSkillIndexLine         = 1400
	maxExplicitUserSkills         = 4
	maxMatchedUserSkills          = 2
	maxMatchedUserSkillCharacters = 6000
	maxInjectedUserSkillChars     = 12000
)

const userSkillsReadme = `LemonSSH user skills

Add one folder per skill inside this directory.
Each skill folder must contain a SKILL.md file.

Example layout:
  Skills/
    My Skill/
      SKILL.md

Minimal SKILL.md:
  ---
  name: My Skill
  description: Short summary of what this skill helps with.
  ---

  Write the skill instructions here.
`

type UserSkillStatusItem struct {
	ID            string   `json:"id"`
	Slug          string   `json:"slug"`
	DirectoryName string   `json:"directoryName"`
	DirectoryPath string   `json:"directoryPath"`
	SkillPath     string   `json:"skillPath"`
	Name          string   `json:"name"`
	Description   string   `json:"description"`
	Status        string   `json:"status"`
	Warnings      []string `json:"warnings"`
	body          string
}

type UserSkillsStatusResult struct {
	OK            bool                  `json:"ok"`
	DirectoryPath string                `json:"directoryPath,omitempty"`
	ReadyCount    int                   `json:"readyCount,omitempty"`
	WarningCount  int                   `json:"warningCount,omitempty"`
	Skills        []UserSkillStatusItem `json:"skills,omitempty"`
	Warnings      []string              `json:"warnings,omitempty"`
	Error         string                `json:"error,omitempty"`
}

type UserSkillsContextResult struct {
	OK      bool   `json:"ok"`
	Context string `json:"context,omitempty"`
	Error   string `json:"error,omitempty"`
}

type UserSkillsService struct{ directory string }

func newUserSkillsService(directory string) *UserSkillsService {
	return &UserSkillsService{directory: directory}
}

func stripSkillQuotes(value string) string {
	value = strings.TrimSpace(value)
	if len(value) >= 2 && ((value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'')) {
		return value[1 : len(value)-1]
	}
	return value
}

func slugifyUserSkill(value string) string {
	var result strings.Builder
	dash := false
	for _, character := range strings.ToLower(strings.TrimSpace(value)) {
		if character >= 'a' && character <= 'z' || character >= '0' && character <= '9' {
			if dash && result.Len() > 0 {
				result.WriteByte('-')
			}
			dash = false
			result.WriteRune(character)
		} else {
			dash = result.Len() > 0
		}
	}
	return strings.Trim(result.String(), "-")
}

func parseSkillFrontmatter(content string) (map[string]string, string, bool) {
	normalized := strings.ReplaceAll(content, "\r\n", "\n")
	if !strings.HasPrefix(normalized, "---\n") {
		return map[string]string{}, content, false
	}
	end := strings.Index(normalized[4:], "\n---")
	if end < 0 {
		return map[string]string{}, content, false
	}
	end += 4
	attributes := map[string]string{}
	for _, rawLine := range strings.Split(normalized[4:end], "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if colon := strings.IndexByte(line, ':'); colon > 0 {
			attributes[strings.TrimSpace(line[:colon])] = stripSkillQuotes(line[colon+1:])
		}
	}
	bodyStart := end + len("\n---")
	for bodyStart < len(normalized) && (normalized[bodyStart] == '\r' || normalized[bodyStart] == '\n') {
		bodyStart++
	}
	return attributes, normalized[bodyStart:], true
}

func (s *UserSkillsService) ensureDirectory() error {
	if err := os.MkdirAll(s.directory, 0o700); err != nil {
		return err
	}
	entries, err := os.ReadDir(s.directory)
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		return os.WriteFile(filepath.Join(s.directory, userSkillsReadmeName), []byte(userSkillsReadme), 0o600)
	}
	return nil
}

func (s *UserSkillsService) scan() (UserSkillsStatusResult, []UserSkillStatusItem) {
	status := UserSkillsStatusResult{OK: true, DirectoryPath: s.directory, Skills: []UserSkillStatusItem{}, Warnings: []string{}}
	if err := s.ensureDirectory(); err != nil {
		status.OK, status.Error = false, err.Error()
		return status, nil
	}
	entries, err := os.ReadDir(s.directory)
	if err != nil {
		status.OK, status.Error = false, err.Error()
		return status, nil
	}
	sort.Slice(entries, func(i, j int) bool { return strings.ToLower(entries[i].Name()) < strings.ToLower(entries[j].Name()) })
	for _, entry := range entries {
		if entry.Type()&os.ModeSymlink != 0 || !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if name == "." || name == ".." || strings.ContainsAny(name, `/\\`) {
			continue
		}
		directoryPath := filepath.Join(s.directory, name)
		skillPath := filepath.Join(directoryPath, "SKILL.md")
		item := UserSkillStatusItem{ID: name, Slug: slugifyUserSkill(name), DirectoryName: name, DirectoryPath: directoryPath, SkillPath: skillPath, Name: name, Status: "warning", Warnings: []string{}}
		info, statErr := os.Lstat(skillPath)
		if statErr != nil {
			item.Warnings = append(item.Warnings, "Missing SKILL.md")
			status.Warnings = append(status.Warnings, name+": Missing SKILL.md")
			status.Skills = append(status.Skills, item)
			continue
		}
		if info.Mode()&os.ModeSymlink != 0 {
			item.Warnings = append(item.Warnings, "SKILL.md must not be a symbolic link.")
		} else if !info.Mode().IsRegular() {
			item.Warnings = append(item.Warnings, "SKILL.md must be a regular file.")
		} else if info.Size() > maxUserSkillBytes {
			item.Warnings = append(item.Warnings, fmt.Sprintf("SKILL.md is too large (%d bytes > %d bytes).", info.Size(), maxUserSkillBytes))
		} else if data, readErr := os.ReadFile(skillPath); readErr != nil {
			item.Warnings = append(item.Warnings, fmt.Sprintf("Failed to read SKILL.md (%v).", readErr))
		} else {
			attributes, body, hasFrontmatter := parseSkillFrontmatter(string(data))
			frontmatterName := strings.TrimSpace(attributes["name"])
			description := strings.TrimSpace(attributes["description"])
			item.Name = frontmatterName
			if item.Name == "" {
				item.Name = name
			}
			item.Description = description
			item.Slug = slugifyUserSkill(item.Name)
			item.body = body
			if !hasFrontmatter {
				item.Warnings = append(item.Warnings, "Missing YAML frontmatter.")
			}
			if frontmatterName == "" {
				item.Warnings = append(item.Warnings, "Missing frontmatter field: name.")
			}
			if description == "" {
				item.Warnings = append(item.Warnings, "Missing frontmatter field: description.")
			} else if len([]rune(description)) > maxUserSkillDescription {
				item.Warnings = append(item.Warnings, fmt.Sprintf("Description is too long (%d chars > %d).", len([]rune(description)), maxUserSkillDescription))
			}
			if item.Slug == "" {
				item.Warnings = append(item.Warnings, "Skill name must include ASCII letters or digits to generate a usable slug.")
			}
		}
		if len(item.Warnings) == 0 {
			item.Status = "ready"
		} else {
			for _, warning := range item.Warnings {
				status.Warnings = append(status.Warnings, name+": "+warning)
			}
		}
		status.Skills = append(status.Skills, item)
	}
	bySlug := map[string][]int{}
	for index, skill := range status.Skills {
		if skill.Status == "ready" && skill.Slug != "" {
			bySlug[skill.Slug] = append(bySlug[skill.Slug], index)
		}
	}
	for slug, indexes := range bySlug {
		if len(indexes) < 2 {
			continue
		}
		warning := fmt.Sprintf("Duplicate skill slug %q. Rename the skill or change its frontmatter name.", slug)
		for _, index := range indexes {
			status.Skills[index].Status = "warning"
			status.Skills[index].Warnings = append(status.Skills[index].Warnings, warning)
			status.Warnings = append(status.Warnings, status.Skills[index].DirectoryName+": "+warning)
		}
	}
	ready := make([]UserSkillStatusItem, 0, len(status.Skills))
	for _, skill := range status.Skills {
		if skill.Status == "ready" {
			status.ReadyCount++
			ready = append(ready, skill)
		} else {
			status.WarningCount++
		}
	}
	return status, ready
}

func (s *UserSkillsService) GetStatus() UserSkillsStatusResult {
	status, _ := s.scan()
	return status
}

func (s *UserSkillsService) OpenFolder() UserSkillsStatusResult {
	status, _ := s.scan()
	if !status.OK {
		return status
	}
	if err := openSystemFile(s.directory); err != nil {
		status.OK, status.Error = false, err.Error()
	}
	return status
}

var userSkillTokenPattern = regexp.MustCompile(`[A-Za-z0-9]+`)

func userSkillTokens(value string) map[string]bool {
	stopwords := map[string]bool{"the": true, "and": true, "for": true, "with": true, "that": true, "this": true, "from": true, "into": true, "when": true, "then": true, "only": true, "your": true, "will": true, "should": true, "have": true, "has": true, "had": true, "using": true, "use": true, "agent": true, "skill": true, "skills": true, "task": true, "file": true, "files": true, "user": true, "about": true}
	result := map[string]bool{}
	for _, token := range userSkillTokenPattern.FindAllString(strings.ToLower(value), -1) {
		if len(token) >= 3 && !stopwords[token] {
			result[token] = true
		}
	}
	return result
}

func truncateSkillText(value string, maximum int) string {
	value = strings.Join(strings.Fields(value), " ")
	characters := []rune(value)
	if len(characters) <= maximum {
		return value
	}
	return strings.TrimSpace(string(characters[:maximum-3])) + "..."
}

func scoreUserSkill(prompt string, skill UserSkillStatusItem) int {
	plainPrompt := " " + strings.ToLower(strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return r
		}
		return ' '
	}, prompt)) + " "
	for _, phrase := range []string{skill.Name, skill.DirectoryName} {
		phrase = strings.Join(strings.Fields(strings.ToLower(phrase)), " ")
		if phrase != "" && strings.Contains(plainPrompt, " "+phrase+" ") {
			return 50
		}
	}
	promptTokens := userSkillTokens(prompt)
	score := 0
	for token := range userSkillTokens(skill.Name + " " + skill.Description) {
		if promptTokens[token] {
			score++
		}
	}
	return score
}

func summarizeUserSkillSlugs(skills []UserSkillStatusItem) string {
	values := make([]string, 0, len(skills))
	for _, skill := range skills {
		values = append(values, "/"+skill.Slug)
	}
	if len(values) > 4 {
		return strings.Join(values[:4], ", ") + fmt.Sprintf(", and %d more", len(values)-4)
	}
	return strings.Join(values, ", ")
}

func (s *UserSkillsService) BuildContext(prompt string, selectedSkillSlugs []string) UserSkillsContextResult {
	status, ready := s.scan()
	if !status.OK {
		return UserSkillsContextResult{Error: status.Error}
	}
	if len(ready) == 0 {
		return UserSkillsContextResult{OK: true}
	}
	indexEntries := []string{}
	indexChars := 0
	remainingCount := 0
	for index, skill := range ready {
		if index >= maxUserSkillIndex {
			remainingCount++
			continue
		}
		entry := skill.Name + ": " + truncateSkillText(skill.Description, maxUserSkillIndexDescription)
		separator := 0
		if len(indexEntries) > 0 {
			separator = 2
		}
		if indexChars+separator+len([]rune(entry)) > maxUserSkillIndexLine {
			remainingCount += len(ready) - index
			break
		}
		indexEntries = append(indexEntries, entry)
		indexChars += separator + len([]rune(entry))
	}
	parts := []string{"User-managed skills are installed in LemonSSH.", "Available user skills: " + strings.Join(indexEntries, "; "), "Use a user-managed skill only when it clearly matches the current request."}
	if remainingCount > 0 {
		parts[1] += fmt.Sprintf("; and %d more.", remainingCount)
	} else {
		parts[1] += "."
	}
	readyBySlug := map[string]UserSkillStatusItem{}
	for _, skill := range ready {
		readyBySlug[skill.Slug] = skill
	}
	selected := []UserSkillStatusItem{}
	unavailable := []string{}
	seen := map[string]bool{}
	for _, rawSlug := range selectedSkillSlugs {
		slug := slugifyUserSkill(rawSlug)
		if slug == "" || seen[slug] {
			continue
		}
		seen[slug] = true
		if len(seen) > maxExplicitUserSkills {
			continue
		}
		if skill, ok := readyBySlug[slug]; ok {
			selected = append(selected, skill)
		} else {
			unavailable = append(unavailable, slug)
		}
	}
	if len(unavailable) > 0 {
		formatted := make([]string, len(unavailable))
		for index, slug := range unavailable {
			formatted[index] = "/" + slug
		}
		parts = append(parts, "The user explicitly selected these LemonSSH user skills for this request, but their content is currently unavailable: "+strings.Join(formatted, ", ")+".")
	}
	type scoredSkill struct {
		skill UserSkillStatusItem
		score int
	}
	matched := []scoredSkill{}
	for _, skill := range ready {
		if seen[skill.Slug] {
			continue
		}
		if score := scoreUserSkill(prompt, skill); score >= 2 {
			matched = append(matched, scoredSkill{skill: skill, score: score})
		}
	}
	sort.SliceStable(matched, func(i, j int) bool { return matched[i].score > matched[j].score })
	for index := 0; index < len(matched) && index < maxMatchedUserSkills; index++ {
		selected = append(selected, matched[index].skill)
	}
	if len(selected) > 0 {
		parts = append(parts, "Matched user-managed skills for this request:")
		remaining := maxInjectedUserSkillChars
		omitted := []UserSkillStatusItem{}
		for _, skill := range selected {
			heading := "### " + skill.Name + "\n"
			body := strings.TrimSpace(skill.body)
			allowed := maxMatchedUserSkillCharacters
			if remaining-len([]rune(heading)) < allowed {
				allowed = remaining - len([]rune(heading))
			}
			if allowed <= 0 || body == "" {
				omitted = append(omitted, skill)
				continue
			}
			bodyRunes := []rune(body)
			if len(bodyRunes) > allowed {
				bodyRunes = bodyRunes[:allowed]
			}
			parts = append(parts, heading+string(bodyRunes))
			remaining -= len([]rune(heading)) + len(bodyRunes)
			if len([]rune(body)) > len(bodyRunes) {
				parts = append(parts, "Some matched user-managed skill content was truncated to stay within the prompt budget: /"+skill.Slug+".")
				break
			}
		}
		if len(omitted) > 0 {
			parts = append(parts, "Additional matched user-managed skills were omitted to stay within the prompt budget: "+summarizeUserSkillSlugs(omitted)+".")
		}
	}
	return UserSkillsContextResult{OK: true, Context: strings.Join(parts, "\n\n")}
}
