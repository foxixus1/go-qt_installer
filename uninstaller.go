package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/therecipe/qt/core"
	"github.com/therecipe/qt/gui"
	"github.com/therecipe/qt/widgets"
)

type InstallInfo struct {
	GameName      string    `json:"game_name"`
	InstallPath   string    `json:"install_path"`
	InstallDate   time.Time `json:"install_date"`
	DesktopFile   string    `json:"desktop_file"`
	MenuFile      string    `json:"menu_file"`
	InstallerPath string    `json:"installer_path"`
	InstallerDir  string    `json:"installer_dir"`
}

var (
	window          *widgets.QMainWindow
	gamesList       *widgets.QListWidget
	uninstallButton *widgets.QPushButton
	infoLabel       *widgets.QLabel
	progressBar     *widgets.QProgressBar
)

func findInstallInfoFiles() []string {
	uninstallerPath, err := os.Executable()
	if err != nil {
		log.Printf("Ошибка при получении пути к деинсталлятору: %v", err)
		return nil
	}
	uninstallerDir := filepath.Dir(uninstallerPath)
	logsDir := filepath.Join(uninstallerDir, "logs")

	if _, err := os.Stat(logsDir); os.IsNotExist(err) {
		log.Printf("Директория с логами не найдена: %s", logsDir)
		return nil
	}

	var infoFiles []string
	files, err := ioutil.ReadDir(logsDir)
	if err != nil {
		log.Printf("Ошибка при чтении директории с логами: %v", err)
		return nil
	}

	for _, file := range files {
		if !file.IsDir() && strings.HasSuffix(file.Name(), "-install.json") {
			infoFiles = append(infoFiles, filepath.Join(logsDir, file.Name()))
		}
	}
	return infoFiles
}

func loadInstallInfo(filePath string) (*InstallInfo, error) {
	data, err := ioutil.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("ошибка при чтении файла %s: %v", filePath, err)
	}
	var info InstallInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return nil, fmt.Errorf("ошибка при разборе JSON: %v", err)
	}
	return &info, nil
}

func uninstallGame(info *InstallInfo) error {
	progressBar.SetRange(0, 4)
	progressBar.SetValue(0)
	progressBar.Show()

	if info.MenuFile != "" {
		if _, err := os.Stat(info.MenuFile); err == nil {
			if err := os.Remove(info.MenuFile); err != nil {
				log.Printf("Ошибка при удалении ярлыка из меню: %v", err)
			}
		}
	}
	progressBar.SetValue(1)

	if info.DesktopFile != "" {
		if _, err := os.Stat(info.DesktopFile); err == nil {
			if err := os.Remove(info.DesktopFile); err != nil {
				log.Printf("Ошибка при удалении ярлыка с рабочего стола: %v", err)
			}
		}
	}
	progressBar.SetValue(2)

	if info.InstallPath != "" {
		if _, err := os.Stat(info.InstallPath); err == nil {
			if err := os.RemoveAll(info.InstallPath); err != nil {
				return fmt.Errorf("ошибка при удалении директории с игрой: %v", err)
			}
		}
	}
	progressBar.SetValue(3)

	exec.Command("gtk-update-icon-cache", "-f", "-t", filepath.Join(os.Getenv("HOME"), ".local", "share", "icons")).Run()
	exec.Command("update-desktop-database", filepath.Join(os.Getenv("HOME"), ".local", "share", "applications")).Run()

	progressBar.SetValue(4)

	infoFilePath := filepath.Join(filepath.Dir(os.Args[0]), "logs", strings.ToLower(info.GameName)+"-install.json")
	if _, err := os.Stat(infoFilePath); err == nil {
		if err := os.Remove(infoFilePath); err != nil {
			log.Printf("Ошибка при удалении файла с информацией об установке: %v", err)
		}
	}

	return nil
}

func updateGamesList() {
	gamesList.Clear()

	infoFiles := findInstallInfoFiles()
	if len(infoFiles) == 0 {
		infoLabel.SetText("Установленные игры не найдены")
		uninstallButton.SetEnabled(false)
		return
	}

	for _, file := range infoFiles {
		info, err := loadInstallInfo(file)
		if err != nil {
			log.Printf("Ошибка при загрузке информации об установке из %s: %v", file, err)
			continue
		}
		installDate := info.InstallDate.Format("02.01.2006 15:04:05")
		item := widgets.NewQListWidgetItem2(fmt.Sprintf("%s (установлена: %s)", info.GameName, installDate), gamesList, 0)
		item.SetData(int(core.Qt__UserRole), core.NewQVariant15(file))
	}

	if gamesList.Count() > 0 {
		gamesList.SetCurrentRow(0)
		uninstallButton.SetEnabled(true)
	} else {
		infoLabel.SetText("Установленные игры не найдены")
		uninstallButton.SetEnabled(false)
	}
}

func main() {
	app := widgets.NewQApplication(len(os.Args), os.Args)

	darkPalette := gui.NewQPalette()
	darkColor := gui.NewQColor3(53, 53, 53, 255)
	whiteColor := gui.NewQColor3(255, 255, 255, 255)
	darkGreyColor := gui.NewQColor3(25, 25, 25, 255)

	darkPalette.SetColor2(gui.QPalette__Window, darkColor)
	darkPalette.SetColor2(gui.QPalette__WindowText, whiteColor)
	darkPalette.SetColor2(gui.QPalette__Base, darkGreyColor)
	darkPalette.SetColor2(gui.QPalette__AlternateBase, darkGreyColor)
	darkPalette.SetColor2(gui.QPalette__ToolTipBase, darkColor)
	darkPalette.SetColor2(gui.QPalette__ToolTipText, whiteColor)
	darkPalette.SetColor2(gui.QPalette__Text, whiteColor)
	darkPalette.SetColor2(gui.QPalette__Button, darkColor)
	darkPalette.SetColor2(gui.QPalette__ButtonText, whiteColor)
	darkPalette.SetColor2(gui.QPalette__BrightText, whiteColor)

	app.SetPalette(darkPalette, "")

	window = widgets.NewQMainWindow(nil, 0)
	window.SetWindowTitle("Деинсталлятор игр")
	window.Resize(core.NewQSize2(500, 400))

	infoLabel = widgets.NewQLabel2("Выберите игру для удаления:", nil, 0)
	gamesList = widgets.NewQListWidget(nil)
	gamesList.ConnectItemClicked(func(item *widgets.QListWidgetItem) {
		uninstallButton.SetEnabled(true)
	})

	progressBar = widgets.NewQProgressBar(nil)
	progressBar.SetTextVisible(true)
	progressBar.SetAlignment(core.Qt__AlignCenter)
	progressBar.Hide()

	uninstallButton = widgets.NewQPushButton2("Удалить выбранную игру", nil)
	uninstallButton.SetEnabled(false)
	uninstallButton.ConnectClicked(func(bool) {
		currentItem := gamesList.CurrentItem()
		if currentItem == nil {
			return
		}

		infoFilePath := currentItem.Data(int(core.Qt__UserRole)).ToString()
		info, err := loadInstallInfo(infoFilePath)
		if err != nil {
			widgets.QMessageBox_Critical(nil, "Ошибка", "Не удалось загрузить информацию об установке: "+err.Error(), widgets.QMessageBox__Ok, widgets.QMessageBox__Ok)
			return
		}

		confirmed := widgets.QMessageBox_Question(nil, "Подтверждение",
			fmt.Sprintf("Вы действительно хотите удалить игру %s?", info.GameName),
			widgets.QMessageBox__Yes|widgets.QMessageBox__No, widgets.QMessageBox__No)

		if confirmed != widgets.QMessageBox__Yes {
			return
		}

		if err := uninstallGame(info); err != nil {
			widgets.QMessageBox_Critical(nil, "Ошибка", "Ошибка при удалении игры: "+err.Error(), widgets.QMessageBox__Ok, widgets.QMessageBox__Ok)
		} else {
			updateGamesList()
		}
	})

	layout := widgets.NewQVBoxLayout()
	layout.AddWidget(infoLabel, 0, 0)
	layout.AddWidget(gamesList, 0, 0)
	layout.AddWidget(progressBar, 0, 0)
	layout.AddWidget(uninstallButton, 0, 0)

	widget := widgets.NewQWidget(nil, 0)
	widget.SetLayout(layout)
	window.SetCentralWidget(widget)

	updateGamesList()
	window.Show()
	app.Exec()
}
