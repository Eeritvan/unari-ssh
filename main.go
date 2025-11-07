package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/signal"
	"slices"
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

var CAMPUSES = [...]string{"Keskusta", "Kumpula", "Meilahti", "Viikki"}
var unicafeData []fetch.Unicafe

var CAMPUS_RESTAURANTS = map[string][]string{
	"Keskusta": {"Myöhä Café & Bar", "Kaivopiha", "Kaisa-talo", "Soc&Kom", "Rotunda", "Porthania Opettajien ravintola", "Porthania", "Topelias", "Olivia", "Metsätalo"},
	"Kumpula":  {"Physicum", "Exactum", "Chemicum", "Chemicum Opettajien ravintola"},
	"Meilahti": {"Terkko", "Meilahti"},
	"Viikki":   {"Tähkä", "Biokeskus 2", "Infokeskus alakerta", "Viikuna", "Infokeskus", "Biokeskus"},
}

const (
	homeView int = iota
	restaurantView
	terminalInfoView
	testingView
	totalViews
)

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	host := os.Getenv("HOST")
	port := os.Getenv("PORT")

	unicafeData, err = fetch.GetUnicafe()
	if err != nil {
		log.Error("Failed to fetch Unicafe data", "error", err)
	}

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
	quitStyle := renderer.NewStyle().Foreground(lipgloss.Color("8"))
	sidebarStyle := renderer.NewStyle().
		Foreground(lipgloss.Color("#04B575")).
		BorderStyle(lipgloss.NormalBorder()).
		BorderRight(true).
		Padding(1, 2).
		Width(20) // TODO: dynamic width
	sidebarItemStyle := renderer.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#FAFAFA")).
		Background(lipgloss.Color("#7D56F4")).
		Padding(1, 2).
		Margin(1).
		Align(lipgloss.Center)
	navStyle := renderer.NewStyle().
		Foreground(lipgloss.Color("12")).
		Italic(true)
	contentStyle := renderer.NewStyle().
		Padding(1, 2)
	bg := "light"
	if renderer.HasDarkBackground() {
		bg = "dark"
	}

	m := model{
		term:             pty.Term,
		profile:          renderer.ColorProfile().Name(),
		width:            pty.Window.Width,
		height:           pty.Window.Height,
		bg:               bg,
		txtStyle:         txtStyle,
		quitStyle:        quitStyle,
		sidebarStyle:     sidebarStyle,
		sidebarItemStyle: sidebarItemStyle,
		navStyle:         navStyle,
		contentStyle:     contentStyle,
		currentView:      homeView,
		data:             unicafeData,
	}
	return m, []tea.ProgramOption{tea.WithAltScreen()}
}

type model struct {
	term             string
	profile          string
	width            int
	height           int
	bg               string
	currentView      int
	txtStyle         lipgloss.Style
	quitStyle        lipgloss.Style
	navStyle         lipgloss.Style
	sidebarStyle     lipgloss.Style
	sidebarItemStyle lipgloss.Style
	contentStyle     lipgloss.Style
	data             []fetch.Unicafe
}

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.height = msg.Height
		m.width = msg.Width
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
		}
	}
	return m, nil
}

func (m model) View() string {
	var content string

	switch m.currentView {
	case homeView:
		content = m.renderRestaurant(m.currentView)
	case restaurantView:
		content = m.renderRestaurant(m.currentView)
	case terminalInfoView:
		content = m.renderRestaurant(m.currentView)
	case testingView:
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

	bottomNav := m.renderBottomNav()

	return lipgloss.JoinVertical(lipgloss.Left, mainView, bottomNav)
}

func (m model) renderRestaurant(idx int) string {
	campus := CAMPUSES[idx]
	campusRestaurants := CAMPUS_RESTAURANTS[campus]

	var restaurantList string
	for _, restaurant := range m.data {
		name := restaurant.Title
		if slices.Contains(campusRestaurants, name) {
			restaurantList += fmt.Sprintf("\n %s -- %v", name, restaurant.Menu.Menus[0].Date)
		}
	}

	content2 := m.txtStyle.Render(restaurantList)

	content := m.txtStyle.Render(campus)
	return content + content2
}

func (m model) renderBottomNav() string {
	bottomNav := m.quitStyle.Render("press 'q' to quit")
	return bottomNav
}

func (m model) renderSidebar() string {
	var campusList []string

	for _, campus := range CAMPUSES {
		sideBarItem := m.sidebarItemStyle.Render(campus)
		campusList = append(campusList, sideBarItem)
	}

	sideBar := lipgloss.JoinVertical(lipgloss.Left, campusList...)
	return sideBar
}
