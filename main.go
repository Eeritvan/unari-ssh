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
	zone "github.com/lrstanley/bubblezone"
)

type unicafeDataMsg []fetch.Unicafe

var LOCATIONS = [...]string{"Keskusta", "Kumpula", "Meilahti", "Töölö", "Viikki"}
var unicafeData []fetch.Unicafe

var LOCATION_RESTAURANTS = map[string][]string{
	"Keskusta": {"Myöhä Café & Bar", "Kaivopiha", "Kaisa-talo", "Soc&Kom", "Rotunda", "Porthania Opettajien ravintola", "Porthania", "Topelias", "Olivia", "Metsätalo"},
	"Kumpula":  {"Physicum", "Exactum", "Chemicum", "Chemicum Opettajien ravintola"},
	"Meilahti": {"Terkko", "Meilahti"},
	"Töölö":    {"Serpens"},
	"Viikki":   {"Tähkä", "Biokeskus 2", "Infokeskus alakerta", "Viikuna", "Infokeskus", "Biokeskus"},
}

const (
	keskustaView int = iota
	kumpulaView
	meilahtiView
	töölöView
	viikkiView
	totalViews
)

func main() {
	err := godotenv.Load()
	if err != nil {
		// TODO: error handling
		//    - must not break dockerfile
		fmt.Println("Error loading .env file")
	}

	host := os.Getenv("HOST")
	port := os.Getenv("PORT")

	zone.NewGlobal()

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
		Padding(1, 2)
	bg := "light"
	if renderer.HasDarkBackground() {
		bg = "dark"
	}

	finland, err := time.LoadLocation("Europe/Helsinki")
	if err != nil {
		// TODO: better error message
		fmt.Println(err)
	}
	currentDate := time.Now().In(finland)

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
		loading:                  true,
	}
	return m, []tea.ProgramOption{tea.WithMouseCellMotion(), tea.WithAltScreen()}
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
	loading                  bool
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
		m.loading = false
		m.data = msg
	case tea.MouseMsg:
		if msg.Action != tea.MouseActionRelease || msg.Button != tea.MouseButtonLeft {
			return m, nil
		}
		for i, campus := range LOCATIONS {
			if m.currentView != i && zone.Get(campus).InBounds(msg) {
				m.currentView = i
			}
		}
		return m, nil
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
			finland, err := time.LoadLocation("Europe/Helsinki")
			if err != nil {
				// TODO: better error message
				fmt.Println(err)
			}
			m.selectedDate = time.Now().In(finland)
		case "ctrl+f":
			// TODO: implement find
			fmt.Println("find")
		}
	}
	return m, nil
}

func (m model) View() string {
	if m.loading {
		return m.renderLoading()
	}
	if m.width < 40 || m.height < 10 {
		return m.renderTermTooSmall()
	}

	sidebar := m.renderSidebar()
	restaurantView := m.renderRestaurant(m.currentView)
	footer := m.renderFooter()

	mainView := lipgloss.JoinHorizontal(lipgloss.Top, sidebar, restaurantView)

	return zone.Scan(lipgloss.JoinVertical(lipgloss.Left, mainView, footer))
}

func (m model) renderRestaurant(idx int) string {
	campus := LOCATIONS[idx]
	campusRestaurants := LOCATION_RESTAURANTS[campus]

	var restaurantList strings.Builder
	restaurantList.WriteString(m.txtStyle.Bold(true).Underline(true).Render(m.selectedDate.Format("Monday 02.01.2006")))
	restaurantList.WriteString("\n")

	found := false
	for _, restaurant := range m.data {
		if slices.Contains(campusRestaurants, restaurant.Title) {
			for _, menu := range restaurant.Menu.Menus {
				restaurantDate := strings.Split(menu.Date, " ")
				currentDate := m.selectedDate.Format("02.01.")
				if restaurantDate[len(restaurantDate)-1] == currentDate {
					found = true
					var menuItems []string
					for _, meal := range menu.Data {
						menuItems = append(menuItems, " • "+strings.Trim(meal.Name, " "))
					}

					restaurantList.WriteString(fmt.Sprintf("\n\n%s\n%s",
						lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12")).Render(restaurant.Title),
						strings.Join(menuItems, "\n")))
				}
			}
		}
	}

	if !found {
		restaurantList.WriteString("\n\nNo data for this date.")
	}

	return m.contentStyle.
		Padding(1, 2).
		BorderStyle(lipgloss.RoundedBorder()).
		Width(m.width - 26).
		Height(m.height - 3).
		MaxHeight(m.height - 1).
		Render(restaurantList.String())
}

// TODO: a: about?
func (m model) renderFooter() string {
	left := m.footerStyle.Render("q: quit")
	right := m.footerStyle.Render("↑/↓: campus\tt: today\t←/→: date")

	leftView := m.footerStyle.Render(left)

	infoWidth := m.width - lipgloss.Width(leftView)

	rightView := m.footerStyle.
		Width(infoWidth).
		Align(lipgloss.Right).
		Render(right)

	return lipgloss.JoinHorizontal(lipgloss.Bottom, leftView, rightView)
}

func (m model) renderSidebar() string {
	var campusList []string

	for i, campus := range LOCATIONS {
		var style lipgloss.Style
		if i == m.currentView {
			style = m.sidebarSelectedItemStyle
		} else {
			style = m.sidebarItemStyle
		}
		sideBarItem := style.Render(campus)

		sideBarItem = zone.Mark(fmt.Sprintf(campus), sideBarItem)

		campusList = append(campusList, sideBarItem)
	}

	sidebarList := lipgloss.JoinVertical(lipgloss.Center, campusList...)

	sidebarStyleWithHeight := m.sidebarStyle.
		Width(22).
		Height(m.height - 3).
		MaxHeight(m.height - 1)

	return sidebarStyleWithHeight.Render(sidebarList)
}

func (m model) renderTermTooSmall() string {
	return lipgloss.NewStyle().
		Width(m.width).
		Height(m.height).
		Align(lipgloss.Center, lipgloss.Center).
		Bold(true).
		Render("Terminal too small")
}

func (m model) renderLoading() string {
	return lipgloss.NewStyle().
		Width(m.width).
		Height(m.height).
		Align(lipgloss.Center, lipgloss.Center).
		Bold(true).
		Render("Loading")
}
