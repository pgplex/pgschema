package include

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Processor handles processing SQL files with \i include directives
type Processor struct {
	baseDir string
	visited map[string]bool
}

// NewProcessor creates a new include processor for the given base directory
func NewProcessor(baseDir string) *Processor {
	return &Processor{
		baseDir: baseDir,
		visited: make(map[string]bool),
	}
}

// ProcessFile processes a SQL file and resolves all \i include directives
func (p *Processor) ProcessFile(filename string) (string, error) {
	// Reset visited map for each top-level file processing
	p.visited = make(map[string]bool)
	
	// Get absolute path to ensure consistent path handling
	absPath, err := filepath.Abs(filename)
	if err != nil {
		return "", fmt.Errorf("failed to get absolute path for %s: %w", filename, err)
	}
	
	// Update base directory based on the input file's directory
	p.baseDir = filepath.Dir(absPath)
	
	return p.processFileRecursive(absPath)
}

// processFileRecursive recursively processes a file and its includes
func (p *Processor) processFileRecursive(filename string) (string, error) {
	// Check for circular dependencies
	if p.visited[filename] {
		return "", fmt.Errorf("circular dependency detected: %s", filename)
	}
	
	// Mark file as visited
	p.visited[filename] = true
	defer func() {
		// Unmark after processing to allow the same file to be included in different branches
		delete(p.visited, filename)
	}()
	
	// Read the file content
	content, err := os.ReadFile(filename)
	if err != nil {
		return "", fmt.Errorf("failed to read file %s: %w", filename, err)
	}
	
	// Process includes in the current file
	currentDir := filepath.Dir(filename)
	processedContent, err := p.processIncludes(string(content), currentDir)
	if err != nil {
		return "", fmt.Errorf("failed to process includes in %s: %w", filename, err)
	}
	
	return processedContent, nil
}

// processIncludes processes \i directives in the given content
func (p *Processor) processIncludes(content string, currentDir string) (string, error) {
	// Regex to match \i directives
	// Matches: \i filename or \i filename; (with optional semicolon)
	includeRegex := regexp.MustCompile(`^\s*\\i\s+([^\s;]+)\s*;?\s*$`)
	
	lines := strings.Split(content, "\n")
	var resultLines []string
	
	for _, line := range lines {
		matches := includeRegex.FindStringSubmatch(line)
		if matches != nil {
			// Found an include directive
			includePath := matches[1]

			// Resolve the include path
			resolvedPath, isFolder, err := p.resolveIncludePath(includePath, currentDir)
			if err != nil {
				return "", fmt.Errorf("failed to resolve include path %s: %w", includePath, err)
			}

			var includedContent string
			if isFolder {
				// Process the folder recursively
				includedContent, err = p.processFolderRecursive(resolvedPath)
				if err != nil {
					return "", fmt.Errorf("failed to process included folder %s: %w", resolvedPath, err)
				}
			} else {
				// Process the included file recursively
				includedContent, err = p.processFileRecursive(resolvedPath)
				if err != nil {
					return "", fmt.Errorf("failed to process included file %s: %w", resolvedPath, err)
				}
			}

			// Split included content into lines and add them
			includedLines := strings.Split(includedContent, "\n")
			// Remove the last empty line if the content ends with \n
			if len(includedLines) > 0 && includedLines[len(includedLines)-1] == "" {
				includedLines = includedLines[:len(includedLines)-1]
			}
			resultLines = append(resultLines, includedLines...)
		} else {
			// Regular line, add as-is
			resultLines = append(resultLines, line)
		}
	}
	
	return strings.Join(resultLines, "\n"), nil
}

// resolveIncludePath resolves an include path relative to the current directory
// Only allows files within the base directory and its subdirectories
// Returns the resolved path and a flag indicating if it's a folder
func (p *Processor) resolveIncludePath(includePath string, currentDir string) (string, bool, error) {
	// Check if this is a folder path (ends with /)
	isFolder := strings.HasSuffix(includePath, "/")

	// Clean the path to remove any . or .. components
	cleanPath := filepath.Clean(includePath)

	// Check for directory traversal attempts
	if strings.Contains(cleanPath, "..") {
		return "", false, fmt.Errorf("directory traversal not allowed: %s", includePath)
	}

	// Resolve relative to current directory
	resolvedPath := filepath.Join(currentDir, cleanPath)

	// Get absolute path
	absPath, err := filepath.Abs(resolvedPath)
	if err != nil {
		return "", false, fmt.Errorf("failed to get absolute path: %w", err)
	}

	// Ensure the resolved path is within the base directory
	baseAbs, err := filepath.Abs(p.baseDir)
	if err != nil {
		return "", false, fmt.Errorf("failed to get absolute base path: %w", err)
	}

	// Check if the resolved path is within the base directory
	relPath, err := filepath.Rel(baseAbs, absPath)
	if err != nil || strings.HasPrefix(relPath, "..") {
		return "", false, fmt.Errorf("include path %s is outside the base directory %s", includePath, p.baseDir)
	}

	// Check if path exists
	stat, err := os.Stat(absPath)
	if os.IsNotExist(err) {
		if isFolder {
			return "", false, fmt.Errorf("included folder does not exist: %s", absPath)
		} else {
			return "", false, fmt.Errorf("included file does not exist: %s", absPath)
		}
	}
	if err != nil {
		return "", false, fmt.Errorf("failed to stat path %s: %w", absPath, err)
	}

	// Validate that the path type matches the expectation
	if isFolder && !stat.IsDir() {
		return "", false, fmt.Errorf("expected folder but found file: %s", absPath)
	}
	if !isFolder && stat.IsDir() {
		return "", false, fmt.Errorf("expected file but found folder: %s (use %s/ for folder includes)", absPath, includePath)
	}

	return absPath, isFolder, nil
}

// processFolderRecursive processes all .sql files in a folder using DFS
func (p *Processor) processFolderRecursive(folderPath string) (string, error) {
	// Read directory contents
	entries, err := os.ReadDir(folderPath)
	if err != nil {
		return "", fmt.Errorf("failed to read directory %s: %w", folderPath, err)
	}

	// Sort entries alphabetically (natural filename order)
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	var resultParts []string

	// Process each entry in alphabetical order
	for _, entry := range entries {
		entryPath := filepath.Join(folderPath, entry.Name())

		if entry.IsDir() {
			// Recursively process subdirectory (DFS)
			subFolderContent, err := p.processFolderRecursive(entryPath)
			if err != nil {
				return "", fmt.Errorf("failed to process subdirectory %s: %w", entryPath, err)
			}
			if subFolderContent != "" {
				resultParts = append(resultParts, subFolderContent)
			}
		} else if strings.HasSuffix(entry.Name(), ".sql") {
			// Process .sql file
			fileContent, err := p.processFileRecursive(entryPath)
			if err != nil {
				return "", fmt.Errorf("failed to process file %s: %w", entryPath, err)
			}
			if fileContent != "" {
				// Ensure the file content ends with a newline for proper concatenation
				if !strings.HasSuffix(fileContent, "\n") {
					fileContent += "\n"
				}
				resultParts = append(resultParts, fileContent)
			}
		}
		// Ignore non-.sql files
	}

	return strings.Join(resultParts, ""), nil
}