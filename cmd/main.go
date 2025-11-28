package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

    "go/ast"
    "go/parser"
    "go/token"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
	"test/pkg/template"
)

// Data holds the user inputs for template generation.
type Data struct {
	ProjectName string
	Options     map[string]bool
}

// model represents the TUI model.
type model struct {
	questions     []string
	answers       []interface{} // []bool for yes/no, string for input
	cursor        int
	confirmed     bool
	isConfirming  bool
	confirmAnswer bool
	textInput     textinput.Model
	viewport      viewport.Model
	err           error
}

var (
	style = lipgloss.NewStyle().Margin(1, 2)
	headerStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#00FF00"))
	selectedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF")).Bold(true)
	yesStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF00"))
	noStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF0000"))
	createStyle = lipgloss.NewStyle().Background(lipgloss.Color("#FFFF00")).Foreground(lipgloss.Color("#000000"))
	instructionStyle = lipgloss.NewStyle().Italic(true).Foreground(lipgloss.Color("#AAAAAA"))
)

func initialModel(questions []string) model {
	ti := textinput.New()
	ti.Focus()
	ti.CharLimit = 156
	ti.Width = 20
	ti.Prompt = "> "

	answers := make([]interface{}, len(questions)-1)
	answers[0] = "" // string for project name
	for i := 1; i < len(questions)-1; i++ {
		answers[i] = false // bool for yes/no
	}

	return model{
		questions:     questions,
		answers:       answers,
		textInput:     ti,
		viewport:      viewport.Model{},
		confirmAnswer: false,
	}
}

func (m model) Init() tea.Cmd {
	return textinput.Blink
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "up":
			if m.cursor == 0 {
				m.answers[0] = m.textInput.Value()
			}
			if m.cursor > 0 {
				m.cursor--
			}
		case "down":
			if m.cursor == 0 {
				m.answers[0] = m.textInput.Value()
			}
			if m.cursor < len(m.questions)-1 {
				m.cursor++
			}
		case "tab":
			if m.cursor == 0 {
				m.answers[0] = m.textInput.Value()
			}
			if m.cursor < len(m.questions)-1 {
				m.cursor++
			}
		case "left":
			if m.cursor > 0 && m.cursor < len(m.questions)-1 {
				m.answers[m.cursor] = false
			}
		case "right":
			if m.cursor > 0 && m.cursor < len(m.questions)-1 {
				m.answers[m.cursor] = true
			}
		case "enter":
			m.answers[0] = m.textInput.Value()
			if m.cursor == len(m.questions)-1 {
				if m.answers[0].(string) == "" {
					// Do not proceed if project name is empty
					return m, nil
				}
				m.confirmed = true
				return m, tea.Quit
			} else {
				if m.cursor < len(m.questions)-1 {
					m.cursor++
				}
			}
		case "y":
			if m.cursor > 0 && m.cursor < len(m.questions)-1 {
				m.answers[m.cursor] = true
			}
		case "n":
			if m.cursor > 0 && m.cursor < len(m.questions)-1 {
				m.answers[m.cursor] = false
			}
		default:
			if m.cursor == 0 {
				m.textInput, cmd = m.textInput.Update(msg)
				return m, cmd
			}
		}
	case tea.WindowSizeMsg:
		m.viewport.Width = msg.Width
		m.viewport.Height = msg.Height - 2 // for header/footer
	}

	if m.cursor == 0 {
		m.textInput.Focus()
	} else {
		m.textInput.Blur()
	}

	return m, nil
}

func (m model) View() string {
	if m.confirmed {
		return "Generating project...\n"
	}

	s := headerStyle.Render("Microservice Generator") + "\n\n"

	for i, q := range m.questions {
		cursor := "  "
		if m.cursor == i {
			cursor = selectedStyle.Render("> ")
		}

		if i == 0 {
			val := m.answers[0].(string)
			if m.cursor == i {
				val = m.textInput.View()
			} else {
				val = selectedStyle.Render(val)
			}
			s += fmt.Sprintf("%s%s %s\n", cursor, q, val)
		} else if i < len(m.questions)-1 {
			valStr := noStyle.Render("No")
			if m.answers[i].(bool) {
				valStr = yesStyle.Render("Yes")
			}
			if m.cursor == i {
				q = selectedStyle.Render(q)
			}
			s += fmt.Sprintf("%s%s %s\n", cursor, q, valStr)
		} else {
			create := "Create"
			if m.cursor == i {
				create = createStyle.Render("Create")
			}
			s += fmt.Sprintf("%s%s\n", cursor, create)
		}
	}

	s += "\n" + instructionStyle.Render("Use up/down arrows or tab to navigate. For Yes/No: y/n or left/right arrows. Enter to select/create. q to quit.") + "\n"

	return style.Render(s)
}



// evaluateComplexCondition supports AND/OR and parentheses.
func evaluateComplexCondition(expr string, data Data) bool {
	tokens := strings.Fields(expr)

	// Replace field names with true/false
	for i, t := range tokens {
		if val, ok := data.Options[t]; ok {
			tokens[i] = fmt.Sprintf("%v", val)
		}
	}

	// Join back
	boolExpr := strings.Join(tokens, " ")

	// Replace AND/OR
	boolExpr = strings.ReplaceAll(boolExpr, "AND", "&&")
	boolExpr = strings.ReplaceAll(boolExpr, "OR", "||")

	// Minimal evaluator
	result, err := evalBooleanExpression(boolExpr)
	if err != nil {
		return false
	}
	return result
}

// evalExpr evaluates an AST expression to a boolean value.
func evalExpr(n ast.Expr) bool {
	switch e := n.(type) {
	case *ast.Ident:
		return e.Name == "true"
	case *ast.BinaryExpr:
		left := evalExpr(e.X)
		right := evalExpr(e.Y)
		switch e.Op {
		case token.LAND:
			return left && right
		case token.LOR:
			return left || right
		}
	case *ast.ParenExpr:
		return evalExpr(e.X)
	}
	return false
}

// evalBooleanExpression evaluates a boolean expression with &&, || and parentheses.
func evalBooleanExpression(expr string) (bool, error) {
	expr = strings.TrimSpace(expr)

	// Use Go parser by embedding into a dummy expression
	code := fmt.Sprintf("package main; func f() bool { return %s }", expr)

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "", code, 0)
	if err != nil {
		return false, err
	}

	var val bool
	ast.Inspect(f, func(n ast.Node) bool {
		if ret, ok := n.(*ast.ReturnStmt); ok {
			if len(ret.Results) > 0 {
				val = evalExpr(ret.Results[0])
			}
		}
		return true
	})

	return val, nil
}

type Config struct {
	Configuration struct {
		Rules []struct {
			Question string `yaml:"question"`
			Variable string `yaml:"variable"`
		} `yaml:"rules"`
	} `yaml:"configuration"`
	Template struct {
		Rules struct {
			Always      []string `yaml:"always"`
			Conditional []struct {
				Files []string `yaml:"files"`
				When  string   `yaml:"when"`
			} `yaml:"conditional"`
		} `yaml:"rules"`
	} `yaml:"template"`
}

// generateProject generates the project files based on data and template path.
func generateProject(data Data, templatePath, projectDir string, config Config) error {
	// Create project directory
	actualProjectDir := filepath.Join(projectDir, data.ProjectName)
	var projectExists bool
	if _, err := os.Stat(actualProjectDir); err == nil {
		projectExists = true
	}
	if err := os.MkdirAll(actualProjectDir, 0755); err != nil {
		return err
	}

	// Use external template directory
	fi, err := os.Stat(templatePath)
	if err != nil {
		return err
	}
	if !fi.IsDir() {
		return fmt.Errorf("template-path must be a directory")
	}

	var sourceFiles []string
	addFiles := func(patterns []string) error {
		for _, pat := range patterns {
			if strings.HasSuffix(pat, "/**") {
				base := strings.TrimSuffix(pat, "/**")
				basePath := filepath.Join(templatePath, base)
				err := filepath.Walk(basePath, func(path string, info os.FileInfo, err error) error {
					if err != nil {
						return err
					}
					if info.IsDir() {
						return nil
					}
					sourceFiles = append(sourceFiles, path)
					return nil
				})
				if err != nil {
					return err
				}
			} else {
				matches, err := filepath.Glob(filepath.Join(templatePath, pat))
				if err != nil {
					return err
				}
				for _, match := range matches {
					info, err := os.Stat(match)
					if err != nil {
						return err
					}
					if !info.IsDir() {
						sourceFiles = append(sourceFiles, match)
					}
				}
			}
		}
		return nil
	}

	if err := addFiles(config.Template.Rules.Always); err != nil {
		return err
	}

	for _, cond := range config.Template.Rules.Conditional {
		if evaluateComplexCondition(cond.When, data) {
			if err := addFiles(cond.Files); err != nil {
				return err
			}
		}
	}

	for _, src := range sourceFiles {
		relPath, err := filepath.Rel(templatePath, src)
		if err != nil {
			return err
		}

		if projectExists && strings.HasPrefix(relPath, "helm/values/") {
			continue
		}

		relPath = strings.ReplaceAll(relPath, "<CHARTNAME>", data.ProjectName)
		relPath = strings.ReplaceAll(relPath, "project_name", data.ProjectName)

		targetPath := filepath.Join(actualProjectDir, relPath)

		if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
			return err
		}

		content, err := os.ReadFile(src)
		if err != nil {
			return err
		}

		contentStr := string(content)
		contentStr = strings.ReplaceAll(contentStr, "<CHARTNAME>", data.ProjectName)
		contentStr = strings.ReplaceAll(contentStr, "project_name", data.ProjectName)

		contentStr = templateengine.RenderTemplate(contentStr, data)

		if err := os.WriteFile(targetPath, []byte(contentStr), 0644); err != nil {
			return err
		}
	}

	fmt.Printf("Project %s generated successfully!\n", data.ProjectName)
	return nil
}

func runTUI(config Config) (Data, error) {
	questions := []string{"Enter project name:"}
	for _, rule := range config.Configuration.Rules {
		questions = append(questions, rule.Question)
	}
	questions = append(questions, "Create")

	p := tea.NewProgram(initialModel(questions), tea.WithAltScreen())
	m, err := p.Run()
	if err != nil {
		return Data{}, err
	}

	md := m.(model)
	if !md.confirmed {
		return Data{}, fmt.Errorf("generation cancelled")
	}

	var data Data
	data.ProjectName = md.answers[0].(string)
	data.Options = make(map[string]bool)
	for i, rule := range config.Configuration.Rules {
		data.Options[rule.Variable] = md.answers[i+1].(bool)
	}

	if data.ProjectName == "" {
		return Data{}, fmt.Errorf("project name is required")
	}

	return data, nil
}

func main() {
	var templatePath string
	var projectDir string
	var configPath string

	var rootCmd = &cobra.Command{
		Use:   "microservice-generator",
		Short: "Generates a microservice template based on user inputs via TUI",
		RunE: func(cmd *cobra.Command, args []string) error {
			if projectDir == "" {
				return cmd.Help()
			}

			if configPath == "" {
				configPath = "./config.yaml"
			}
			configContent, err := os.ReadFile(configPath)
			if err != nil {
				return err
			}

			var config Config
			if err := yaml.Unmarshal(configContent, &config); err != nil {
				return err
			}

			data, err := runTUI(config)
			if err != nil {
				fmt.Printf("Error: %v\n", err)
				os.Exit(1)
			}

			if err := generateProject(data, templatePath, projectDir, config); err != nil {
				fmt.Printf("Error generating project: %v\n", err)
				os.Exit(1)
			}
			return nil
		},
	}

	rootCmd.Flags().StringVarP(&templatePath, "template-path", "t", "", "Path to the template directory (required)")
	rootCmd.Flags().StringVarP(&projectDir, "project-dir", "p", "", "Path to the project directory")
	rootCmd.Flags().StringVarP(&configPath, "config", "c", "", "Path to the config file")
	rootCmd.MarkFlagRequired("template-path")

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}