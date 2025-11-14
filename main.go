package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/signal"
	"slices"
	"strings"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/log"
	"github.com/charmbracelet/ssh"
	"github.com/charmbracelet/wish"
	"github.com/charmbracelet/wish/activeterm"
	"github.com/charmbracelet/wish/bubbletea"
	"github.com/charmbracelet/wish/logging"
	"github.com/eeritvan/unari-ssh/pkg/fetch"
	"github.com/joho/godotenv"
)

type unicafeDataMsg []fetch.Unicafe

var CAMPUSES = [...]string{"Keskusta", "Kumpula", "Meilahti", "Viikki"}
var unicafeData []fetch.Unicafe

var CAMPUS_RESTAURANTS = map[string][]string{
	"Keskusta": {"Myöhä Café & Bar", "Kaivopiha", "Kaisa-talo", "Soc&Kom", "Rotunda", "Porthania Opettajien ravintola", "Porthania", "Topelias", "Olivia", "Metsätalo"},
	"Kumpula":  {"Physicum", "Exactum", "Chemicum", "Chemicum Opettajien ravintola"},
	"Meilahti": {"Terkko", "Meilahti"},
	"Viikki":   {"Tähkä", "Biokeskus 2", "Infokeskus alakerta", "Viikuna", "Infokeskus", "Biokeskus"},
}

const (
	keskustaView int = iota
	kumpulaView
	meilahtiView
	viikkiView
	totalViews
)

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	host := os.Getenv("HOST")
	port := os.Getenv("PORT")

	s, err := wish.NewServer(
		wish.WithAddress(net.JoinHostPort(host, port)),
		wish.WithHostKeyPath(".ssh/id_ed25519"),
		wish.WithMiddleware(
			bubbletea.Middleware(teaHandler),
			activeterm.Middleware(),
			logging.Middleware(),
		),
	)
	if err != nil {
		log.Error("Could not start server", "error", err)
	}

	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	log.Info("Starting SSH server", "host", host, "port", port)
	go func() {
		if err = s.ListenAndServe(); err != nil && !errors.Is(err, ssh.ErrServerClosed) {
			log.Error("Could not start server", "error", err)
			done <- nil
		}
	}()

	<-done
	log.Info("Stopping SSH server")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer func() { cancel() }()
	if err := s.Shutdown(ctx); err != nil && !errors.Is(err, ssh.ErrServerClosed) {
		log.Error("Could not stop server", "error", err)
	}
}

func teaHandler(s ssh.Session) (tea.Model, []tea.ProgramOption) {
	pty, _, _ := s.Pty()

	renderer := bubbletea.MakeRenderer(s)
	txtStyle := renderer.NewStyle().Foreground(lipgloss.Color("10"))
	sidebarStyle := renderer.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		Foreground(lipgloss.Color("#04B575")).
		Padding(1, 2).
		Width(20)
	sidebarItemStyle := renderer.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#FAFAFA")).
		Background(lipgloss.Color("#7D56F4")).
		Padding(1, 2).
		Margin(1, 0).
		Width(16).
		Align(lipgloss.Center)
	sidebarSelectedItemStyle := renderer.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#000000")).
		Background(lipgloss.Color("#FFFF00")).
		Padding(1, 2).
		Margin(1, 0).
		Width(16).
		Align(lipgloss.Center)
	footerStyle := renderer.NewStyle().
		Bold(true).
		Italic(true).
		TabWidth(4).
		Foreground(lipgloss.Color("#3C3C3C"))
	contentStyle := renderer.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		Padding(1, 2)
	bg := "light"
	if renderer.HasDarkBackground() {
		bg = "dark"
	}

	currentDate := time.Now()

	m := model{
		term:                     pty.Term,
		profile:                  renderer.ColorProfile().Name(),
		width:                    pty.Window.Width,
		height:                   pty.Window.Height,
		bg:                       bg,
		txtStyle:                 txtStyle,
		sidebarStyle:             sidebarStyle,
		sidebarItemStyle:         sidebarItemStyle,
		sidebarSelectedItemStyle: sidebarSelectedItemStyle,
		footerStyle:              footerStyle,
		contentStyle:             contentStyle,
		currentView:              kumpulaView,
		data:                     unicafeData,
		selectedDate:             currentDate,
	}
	return m, []tea.ProgramOption{tea.WithAltScreen()}
}

type model struct {
	term                     string
	profile                  string
	width                    int
	height                   int
	bg                       string
	currentView              int
	txtStyle                 lipgloss.Style
	footerStyle              lipgloss.Style
	sidebarStyle             lipgloss.Style
	sidebarItemStyle         lipgloss.Style
	sidebarSelectedItemStyle lipgloss.Style
	contentStyle             lipgloss.Style
	data                     []fetch.Unicafe
	selectedDate             time.Time
}

func (m model) Init() tea.Cmd {
	return func() tea.Msg {
		data, err := fetch.GetUnicafe()
		if err != nil {
			// TODO: error handling
			return unicafeDataMsg([]fetch.Unicafe{})
		}
		return unicafeDataMsg(data)
	}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.height = msg.Height
		m.width = msg.Width
	case unicafeDataMsg:
		m.data = msg
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "up", "k":
			m.currentView--
			if m.currentView < 0 {
				m.currentView = totalViews - 1
			}
		case "down", "j":
			m.currentView++
			if m.currentView >= totalViews {
				m.currentView = 0
			}
		case "right", "l": // next day
			// TODO: check that unicafe has this date
			m.selectedDate = m.selectedDate.AddDate(0, 0, 1)
		case "left", "h": // prev day
			// TODO: check that unicafe has this date
			m.selectedDate = m.selectedDate.AddDate(0, 0, -1)
		case "t", "T": // current date
			m.selectedDate = time.Now()
		case "ctrl+f":
			// TODO: implement find
			fmt.Println("find")
		}
	}
	return m, nil
}

func (m model) View() string {
	var content string

	switch m.currentView {
	case keskustaView:
		content = m.renderRestaurant(m.currentView)
	case kumpulaView:
		content = m.renderRestaurant(m.currentView)
	case meilahtiView:
		content = m.renderRestaurant(m.currentView)
	case viikkiView:
		content = m.renderRestaurant(m.currentView)
	}

	sideBar := m.renderSidebar()

	mainHeight := m.height - 3

	sidebarStyleWithHeight := m.sidebarStyle.Height(mainHeight)
	contentStyleWithHeight := m.contentStyle.
		Width(m.width - 24).
		Height(mainHeight)

	sidebarContent := sidebarStyleWithHeight.Render(sideBar)
	mainContent := contentStyleWithHeight.Render(content)

	mainView := lipgloss.JoinHorizontal(lipgloss.Top, sidebarContent, mainContent)

	footer := m.renderFooter()

	return lipgloss.JoinVertical(lipgloss.Left, mainView, footer)
}

func (m model) renderRestaurant(idx int) string {
	campus := CAMPUSES[idx]
	campusRestaurants := CAMPUS_RESTAURANTS[campus]

	var restaurantList string
	for _, restaurant := range m.data {
		name := restaurant.Title
		if slices.Contains(campusRestaurants, name) {
			for _, menu := range restaurant.Menu.Menus {
				restaurantDate := strings.Split(menu.Date, " ")
				currentDate := m.selectedDate.Format("2.1.")
				if restaurantDate[len(restaurantDate)-1] == currentDate {
					var test2 []string
					for _, test := range menu.Data {
						test2 = append(test2, test.Name)
					}
					restaurantList += fmt.Sprintf("\n %s -- %v", name, test2)
				}
			}
		}
	}

	content := m.txtStyle.Render(restaurantList)

	return m.txtStyle.Render(m.selectedDate.Format("Monday 2.1")) + content
}

func (m model) renderFooter() string {
	left := m.footerStyle.Render("q: quit")
	right := m.footerStyle.Render("↑/↓: campus\tt: today\t←/→: date")

	rightBox := lipgloss.NewStyle().
		Width(m.width - lipgloss.Width(left)).
		Align(lipgloss.Right).
		Render(right)

	return m.footerStyle.Width(m.width).Render(left + rightBox)
}

func (m model) renderSidebar() string {
	var campusList []string

	for i, campus := range CAMPUSES {
		var style lipgloss.Style
		if i == m.currentView {
			style = m.sidebarSelectedItemStyle
		} else {
			style = m.sidebarItemStyle
		}
		sideBarItem := style.Render(campus)
		campusList = append(campusList, sideBarItem)
	}

	sideBar := lipgloss.JoinVertical(lipgloss.Center, campusList...)
	return sideBar
}
