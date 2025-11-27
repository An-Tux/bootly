package main

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"

    "go/ast"
    "go/parser"
    "go/token"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
	"test/pkg/templateengine"
)

// Data holds the user inputs for template generation.
type Data struct {
	ProjectName     string
	UseREST         bool
	UseGRPC         bool
	UseGraphQL      bool
	UsePostgresMig  bool
	UseMongoMig     bool
	UseCronJobs     bool
	UseWorkers      bool
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
	questions = []string{
		"Enter project name:",
		"Include REST? (Y/N)",
		"Include gRPC? (Y/N)",
		"Include GraphQL? (Y/N)",
		"Include миграции PostgreSQL? (Y/N)",
		"Include миграции MongoDB? (Y/N)",
		"Include cron jobs? (Y/N)",
		"Добавить workers? (Y/N)",
	}
	style = lipgloss.NewStyle().Margin(1, 2)
)

func initialModel() model {
	ti := textinput.New()
	ti.Focus()
	ti.CharLimit = 156
	ti.Width = 20

	answers := make([]interface{}, len(questions))
	answers[0] = "" // string for project name
	for i := 1; i < len(questions); i++ {
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
			if !m.isConfirming && m.cursor > 0 {
				m.cursor--
			}
		case "down":
			if !m.isConfirming && m.cursor < len(m.questions)-1 {
				m.cursor++
			}
		case "tab":
			if !m.isConfirming && m.cursor < len(m.questions)-1 {
				m.cursor++
			}
		case "left":
			if m.isConfirming {
				m.confirmAnswer = false
			} else if m.cursor > 0 {
				m.answers[m.cursor] = false
			}
		case "right":
			if m.isConfirming {
				m.confirmAnswer = true
			} else if m.cursor > 0 {
				m.answers[m.cursor] = true
			}
		case "enter":
			if m.isConfirming {
				if m.confirmAnswer {
					m.confirmed = true
					return m, tea.Quit
				} else {
					m.isConfirming = false
				}
			} else {
				if m.cursor == 0 {
					m.answers[0] = m.textInput.Value()
				}
				if m.answers[0].(string) == "" {
					// Do not proceed if project name is empty
					return m, nil
				}
				m.isConfirming = true
			}
		case "y":
			if m.isConfirming {
				m.confirmAnswer = true
			} else if m.cursor > 0 {
				m.answers[m.cursor] = true
			}
		case "n":
			if m.isConfirming {
				m.confirmAnswer = false
			} else if m.cursor > 0 {
				m.answers[m.cursor] = false
			}
		default:
			if !m.isConfirming && m.cursor == 0 {
				m.textInput, cmd = m.textInput.Update(msg)
				return m, cmd
			}
		}
	case tea.WindowSizeMsg:
		m.viewport.Width = msg.Width
		m.viewport.Height = msg.Height - 2 // for header/footer
	}

	if !m.isConfirming {
		if m.cursor == 0 {
			m.textInput.Focus()
		} else {
			m.textInput.Blur()
		}
	} else {
		m.textInput.Blur()
	}

	return m, nil
}

func (m model) View() string {
	if m.confirmed {
		return "Generating project...\n"
	}

	s := "Microservice Generator\n\n"

	if m.isConfirming {
		// Show filled parameters
		for i, q := range m.questions {
			if i == 0 {
				val := m.answers[0].(string)
				s += fmt.Sprintf("%s %s\n", q, val)
			} else {
				val := "No"
				if m.answers[i].(bool) {
					val = "Yes"
				}
				s += fmt.Sprintf("%s %s\n", q, val)
			}
		}
		s += "\nAll filled correctly? (Y/N) "
		val := "No"
		if m.confirmAnswer {
			val = "Yes"
		}
		s += val + "\n"
		s += "\nPress y/n/left/right to toggle, enter to confirm."
	} else {
		for i, q := range m.questions {
			cursor := " "
			if m.cursor == i {
				cursor = ">"
			}

			if i == 0 {
				val := m.answers[0].(string)
				if val == "" && m.cursor == i {
					val = m.textInput.View()
				}
				s += fmt.Sprintf("%s %s %s\n", cursor, q, val)
			} else {
				val := "No"
				if m.answers[i].(bool) {
					val = "Yes"
				}
				s += fmt.Sprintf("%s %s %s\n", cursor, q, val)
			}
		}

		s += "\nPress q to quit, up/down/tab to navigate, y/n/left/right to set for yes/no.\nPress enter to proceed to confirmation."
	}

	return style.Render(s)
}



// evaluateComplexCondition supports AND/OR and parentheses.
func evaluateComplexCondition(expr string, data Data) bool {
	tokens := strings.Fields(expr)

	// Replace field names with true/false
	for i, t := range tokens {
		field := reflect.ValueOf(data).FieldByName(t)
		if field.IsValid() && field.Kind() == reflect.Bool {
			tokens[i] = fmt.Sprintf("%v", field.Bool())
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
			if lit, ok := ret.Results[0].(*ast.Ident); ok {
				val = (lit.Name == "true")
			}
		}
		return true
	})

	return val, nil
}


// generateProject generates the project files based on data and template path.
func generateProject(data Data, templatePath string) error {
	// Create project directory
	projectDir := data.ProjectName
	if err := os.Mkdir(projectDir, 0755); err != nil {
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

	err = filepath.Walk(templatePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(templatePath, path)
		if err != nil {
			return err
		}

		// Replace custom placeholders like <CHARTNAME> in relPath with ProjectName
		relPath = strings.ReplaceAll(relPath, "<CHARTNAME>", data.ProjectName)
		// Also replace "project_name" in relPath with ProjectName (for renaming folders/files)
		relPath = strings.ReplaceAll(relPath, "project_name", data.ProjectName)

		targetPath := filepath.Join(projectDir, relPath)

		if info.IsDir() {
			return os.MkdirAll(targetPath, 0755)
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		// Replace custom placeholders like <CHARTNAME> in content with ProjectName
		contentStr := strings.ReplaceAll(string(content), "<CHARTNAME>", data.ProjectName)
		// Also replace "project_name" in content with ProjectName
		contentStr = strings.ReplaceAll(contentStr, "project_name", data.ProjectName)

		// Process custom conditional blocks [if condition] ... [endif]
		contentStr = templateengine.RenderTemplate(contentStr, data)

		return os.WriteFile(targetPath, []byte(contentStr), 0644)
	})
	if err != nil {
		return err
	}

	fmt.Printf("Project %s generated successfully!\n", data.ProjectName)
	return nil
}

func runTUI() (Data, error) {
	p := tea.NewProgram(initialModel(), tea.WithAltScreen())
	m, err := p.Run()
	if err != nil {
		return Data{}, err
	}

	md := m.(model)
	if !md.confirmed {
		return Data{}, fmt.Errorf("generation cancelled")
	}

	data := Data{
		ProjectName:    md.answers[0].(string),
		UseREST:        md.answers[1].(bool),
		UseGRPC:        md.answers[2].(bool),
		UseGraphQL:     md.answers[3].(bool),
		UsePostgresMig: md.answers[4].(bool),
		UseMongoMig:    md.answers[5].(bool),
		UseCronJobs:    md.answers[6].(bool),
		UseWorkers:     md.answers[7].(bool),
	}

	if data.ProjectName == "" {
		return Data{}, fmt.Errorf("project name is required")
	}

	return data, nil
}

func main() {
	var templatePath string

	var rootCmd = &cobra.Command{
		Use:   "microservice-generator",
		Short: "Generates a microservice template based on user inputs via TUI",
		Run: func(cmd *cobra.Command, args []string) {
			data, err := runTUI()
			if err != nil {
				fmt.Printf("Error: %v\n", err)
				os.Exit(1)
			}

			if err := generateProject(data, templatePath); err != nil {
				fmt.Printf("Error generating project: %v\n", err)
				os.Exit(1)
			}
		},
	}

	rootCmd.Flags().StringVar(&templatePath, "template-path", "", "Path to the template directory (required)")
	rootCmd.MarkFlagRequired("template-path")

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}