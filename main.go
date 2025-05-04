package main

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/therecipe/qt/core"
	"github.com/therecipe/qt/gui"
	"github.com/therecipe/qt/widgets"
)

// InstallInfo структура для хранения информации об установке
type InstallInfo struct {
	GameName        string    `json:"game_name"`
	InstallPath     string    `json:"install_path"`
	InstallDate     time.Time `json:"install_date"`
	DesktopFile     string    `json:"desktop_file"`
	MenuFile        string    `json:"menu_file"`
	InstallerPath   string    `json:"installer_path"`
	InstallerDir    string    `json:"installer_dir"`
	UninstallerPath string    `json:"uninstaller_path"` // Новое поле для пути к uninstaller
}

type Config struct {
	InstallPath        string             `json:"install_path"`
	IconPath           string             `json:"icon_path"`
	BannerPath         string             `json:"banner_path"`
	GameAssets         []string           `json:"game_assets"`
	DLLPath            string             `json:"dll_path"`
	ExecPath           string             `json:"exec_path"` // Путь к основному исполняемому файлу
	ExecDirs           []string           `json:"exec_dirs"` // Директории, где искать исполняемые файлы
	DesktopEntry       DesktopEntryConfig `json:"desktop_entry"`
	MinRequiredSpaceGB float64            `json:"min_required_space_gb"`
}

type DesktopEntryConfig struct {
	Name       string `json:"name"`
	Exec       string `json:"exec"`
	Icon       string `json:"icon"`
	Categories string `json:"categories"`
	Type       string `json:"type"`
	Terminal   bool   `json:"terminal"`
	Comment    string `json:"comment"`
}

var config Config
var installButton *widgets.QPushButton
var pathLabel *widgets.QLabel
var progressBar *widgets.QProgressBar
var createShortcutCheckBox *widgets.QCheckBox
var installInfo InstallInfo

func loadConfig(filePath string) error {
	data, err := ioutil.ReadFile(filePath)
	if err != nil {
		return err
	}

	return json.Unmarshal(data, &config)
}

func chooseInstallPath() {
	dialog := widgets.QFileDialog_GetExistingDirectory(nil, "Выберите путь установки", "", 0)
	if dialog != "" {
		config.InstallPath = filepath.Join(dialog, "Celeste")
		updateInstallPathDisplay()
		checkInstallButtonState()
	}
}

func updateInstallPathDisplay() {
	pathLabel.SetText("Путь установки: " + config.InstallPath)
}

func checkInstallButtonState() {
	if config.InstallPath != "" {
		installButton.SetEnabled(true)
	} else {
		installButton.SetEnabled(false)
	}
}

func checkDiskSpace() (float64, error) {
	// Check if the directory exists
	dirPath := config.InstallPath

	// If the directory doesn't exist yet, check the parent directory
	if _, err := os.Stat(dirPath); os.IsNotExist(err) {
		dirPath = filepath.Dir(dirPath)

		// If parent directory also doesn't exist, use the current directory
		if _, err := os.Stat(dirPath); os.IsNotExist(err) {
			dirPath = "."
		}
	}

	var stat syscall.Statfs_t
	err := syscall.Statfs(dirPath, &stat)
	if err != nil {
		return 0, err
	}

	freeSpace := stat.Bavail * uint64(stat.Bsize)
	// Convert to gigabytes
	freeSpaceGB := float64(freeSpace) / (1024 * 1024 * 1024)
	return freeSpaceGB, nil
}

// Функция для установки прав на исполнение для файлов
func setExecutablePermissions(path string) error {
	return os.Chmod(path, 0755)
}

// Функция для рекурсивного поиска и установки прав на исполнение для бинарных файлов
func makeFilesExecutable(dir string, patterns []string) {
	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		// Пропускаем директории
		if info.IsDir() {
			return nil
		}

		// Проверяем, является ли файл исполняемым по расширению или имени
		isExecutable := false

		// Проверяем по расширению
		ext := strings.ToLower(filepath.Ext(path))
		if ext == ".sh" || ext == ".bin" || ext == ".x86" || ext == ".x86_64" || ext == "" {
			isExecutable = true
		}

		// Проверяем по имени файла
		baseName := strings.ToLower(filepath.Base(path))
		if strings.Contains(baseName, "run") || strings.Contains(baseName, "start") ||
			strings.Contains(baseName, "game") || strings.Contains(baseName, "celeste") {
			isExecutable = true
		}

		// Проверяем по шаблонам из конфига
		for _, pattern := range patterns {
			if matched, _ := filepath.Match(pattern, filepath.Base(path)); matched {
				isExecutable = true
				break
			}
		}

		if isExecutable {
			log.Printf("Устанавливаем права на исполнение для: %s", path)
			if err := setExecutablePermissions(path); err != nil {
				log.Printf("Ошибка при установке прав на исполнение для %s: %v", path, err)
			}
		}

		return nil
	})
}

// copyFile копирует файл из src в dst и устанавливает права на исполнение
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	if err != nil {
		return err
	}

	return os.Chmod(dst, 0755) // Устанавливаем права на исполнение
}

// saveInstallInfo сохраняет информацию об установке в директории игры
func saveInstallInfo() error {
	logsDir := filepath.Join(config.InstallPath, "logs")
	if err := os.MkdirAll(logsDir, 0755); err != nil {
		return fmt.Errorf("не удалось создать директорию для логов: %v", err)
	}

	gameName := strings.ToLower(config.DesktopEntry.Name)
	gameName = strings.ReplaceAll(gameName, " ", "-")
	infoFilePath := filepath.Join(logsDir, gameName+"-install.json")

	installInfo.GameName = config.DesktopEntry.Name
	installInfo.InstallPath = config.InstallPath
	installInfo.InstallDate = time.Now()
	installInfo.InstallerPath, _ = os.Executable()
	installInfo.InstallerDir = filepath.Dir(installInfo.InstallerPath)
	installInfo.UninstallerPath = filepath.Join(config.InstallPath, "uninstaller")

	data, err := json.MarshalIndent(installInfo, "", "  ")
	if err != nil {
		return fmt.Errorf("ошибка при сериализации информации об установке: %v", err)
	}

	if err := ioutil.WriteFile(infoFilePath, data, 0644); err != nil {
		return fmt.Errorf("ошибка при сохранении информации об установке: %v", err)
	}

	log.Printf("Информация об установке сохранена в %s", infoFilePath)
	return nil
}

func startInstallation() {
	// Блокируем кнопку на время установки и меняем текст
	installButton.SetEnabled(false)
	installButton.SetText("Установка...")

	// Подсчет общего размера файлов для прогрессбара
	totalFiles := 0
	zipFiles := make(map[string]*zip.ReadCloser)

	// Открываем все zip-файлы для подсчета содержимого
	for _, asset := range config.GameAssets {
		r, err := zip.OpenReader(asset)
		if err != nil {
			displayError("Ошибка при открытии архива: " + err.Error())
			installButton.SetEnabled(true)
			installButton.SetText("Начать установку")
			return
		}
		zipFiles[asset] = r
		totalFiles += len(r.File)
	}

	// Если нет файлов для распаковки
	if totalFiles == 0 {
		displayError("Архивы пусты или повреждены")
		installButton.SetEnabled(true)
		installButton.SetText("Начать установку")
		return
	}

	// Проверяем свободное место на диске
	freeSpaceGB, err := checkDiskSpace()
	if err != nil {
		displayError("Ошибка при проверке дискового пространства: " + err.Error())
		installButton.SetEnabled(true)
		installButton.SetText("Начать установку")
		return
	}

	// Проверяем требуемое минимальное пространство из конфигурации
	if freeSpaceGB < config.MinRequiredSpaceGB {
		displayError(fmt.Sprintf("Недостаточно места для установки. Свободно: %.2f ГБ, требуется: %.2f ГБ.",
			freeSpaceGB, config.MinRequiredSpaceGB))
		installButton.SetEnabled(true)
		installButton.SetText("Начать установку")
		return
	}

	// Создаем базовую директорию для установки
	err = os.MkdirAll(config.InstallPath, os.ModePerm)
	if err != nil {
		displayError("Не удалось создать директорию для установки: " + err.Error())
		installButton.SetEnabled(true)
		installButton.SetText("Начать установку")
		return
	}

	// Настраиваем прогрессбар
	progressBar.SetRange(0, totalFiles)
	progressBar.SetValue(0)
	progressBar.Show()

	// Создаем канал для обновления прогрессбара
	updateChan := make(chan int)
	errorChan := make(chan string)
	doneChan := make(chan bool)

	// Обработчик сообщений от горутины установки
	go func() {
		for {
			select {
			case progress := <-updateChan:
				// Обновляем прогрессбар
				progressBar.SetValue(progress)
				progressBar.SetFormat(fmt.Sprintf("%d%% (%d/%d)", progress*100/totalFiles, progress, totalFiles))
			case errMsg := <-errorChan:
				// Показываем сообщение об ошибке
				widgets.QMessageBox_Warning(nil, "Предупреждение", errMsg, widgets.QMessageBox__Ok, widgets.QMessageBox__Ok)
			case <-doneChan:
				// Установка завершена
				progressBar.SetValue(totalFiles)
				progressBar.SetFormat("100% - Установка завершена")
				widgets.QMessageBox_Information(nil, "Установка завершена",
					"Установка игры успешно завершена!", widgets.QMessageBox__Ok, widgets.QMessageBox__Ok)
				installButton.SetEnabled(true)
				installButton.SetText("Начать установку")
				return
			}
		}
	}()

	// Запускаем установку в отдельной горутине
	go func() {
		extractedFiles := 0

		// Распаковка файлов
		for _, asset := range config.GameAssets {
			r := zipFiles[asset]
			defer r.Close()

			for _, f := range r.File {
				fpath := filepath.Join(config.InstallPath, f.Name)

				// Проверка на путь выхода за пределы
				if !strings.HasPrefix(fpath, filepath.Clean(config.InstallPath)+string(os.PathSeparator)) {
					errorChan <- "Обнаружена попытка распаковки за пределы директории установки"
					continue
				}

				// Создаем директории для файлов
				if f.FileInfo().IsDir() {
					os.MkdirAll(fpath, os.ModePerm)
					extractedFiles++
					updateChan <- extractedFiles
					continue
				}

				// Создание директорий для файла, если нет
				if err := os.MkdirAll(filepath.Dir(fpath), os.ModePerm); err != nil {
					errorChan <- "Не удалось создать директорию: " + err.Error()
					continue
				}

				// Создание файла
				outFile, err := os.Create(fpath)
				if err != nil {
					errorChan <- "Не удалось создать файл: " + err.Error()
					continue
				}

				// Копирование содержимого
				rc, err := f.Open()
				if err != nil {
					outFile.Close()
					errorChan <- "Не удалось открыть файл в архиве: " + err.Error()
					continue
				}

				_, err = io.Copy(outFile, rc)
				rc.Close()
				outFile.Close()

				if err != nil {
					errorChan <- "Ошибка копирования данных: " + err.Error()
					continue
				}

				extractedFiles++
				updateChan <- extractedFiles
			}
		}

		// Устанавливаем права на исполнение для основного исполняемого файла
		if config.ExecPath != "" {
			execFullPath := filepath.Join(config.InstallPath, config.ExecPath)
			log.Printf("Устанавливаем права на исполнение для основного файла: %s", execFullPath)

			if err := setExecutablePermissions(execFullPath); err != nil {
				log.Printf("Ошибка при установке прав на исполнение: %v", err)
				errorChan <- "Не удалось установить права на исполнение для игры: " + err.Error()
			} else {
				log.Printf("Права на исполнение успешно установлены для основного файла")
			}
		}

		// Устанавливаем права на исполнение для всех потенциально исполняемых файлов
		log.Printf("Поиск и установка прав на исполнение для всех исполняемых файлов...")

		// Если указаны директории для поиска исполняемых файлов
		if len(config.ExecDirs) > 0 {
			for _, dir := range config.ExecDirs {
				fullDir := filepath.Join(config.InstallPath, dir)
				makeFilesExecutable(fullDir, []string{"*.sh", "*.bin", "*.x86", "*.x86_64"})
			}
		} else {
			// Иначе ищем во всей директории установки
			makeFilesExecutable(config.InstallPath, []string{"*.sh", "*.bin", "*.x86", "*.x86_64"})
		}

		// Копируем uninstaller в директорию игры
		installerDir := filepath.Dir(os.Args[0])
		uninstallerSrc := filepath.Join(installerDir, "uninstaller")
		uninstallerDst := filepath.Join(config.InstallPath, "uninstaller")

		if err := copyFile(uninstallerSrc, uninstallerDst); err != nil {
			log.Printf("Ошибка при копировании деинсталлятора: %v", err)
			errorChan <- "Не удалось скопировать деинсталлятор: " + err.Error()
		} else {
			log.Printf("Деинсталлятор успешно скопирован в %s", uninstallerDst)
		}

		// Создаем ярлык если нужно
		if createShortcutCheckBox.IsChecked() {
			createShortcut()
		}

		// Сохраняем информацию об установке
		if err := saveInstallInfo(); err != nil {
			log.Printf("Ошибка при сохранении информации об установке: %v", err)
		}

		// Сигнализируем о завершении установки
		doneChan <- true
	}()
}

func createShortcut() {
	// Для Linux
	appDir := filepath.Join(os.Getenv("HOME"), ".local", "share", "applications")
	os.MkdirAll(appDir, os.ModePerm)

	// Имя файла .desktop на основе названия приложения
	appName := strings.ToLower(config.DesktopEntry.Name)
	appName = strings.ReplaceAll(appName, " ", "-")
	desktopFile := filepath.Join(appDir, appName+".desktop")

	// Создание исполняемого пути, если в конфиге указана только относительная часть
	execPath := config.DesktopEntry.Exec
	if !filepath.IsAbs(execPath) {
		execPath = filepath.Join(config.InstallPath, execPath)
	}

	// Создание пути к иконке
	iconPath := ""

	// Проверяем, есть ли иконка в конфиге
	if config.DesktopEntry.Icon != "" {
		iconPath = config.DesktopEntry.Icon
		if !filepath.IsAbs(iconPath) {
			iconPath = filepath.Join(config.InstallPath, iconPath)
		}
	} else if config.IconPath != "" {
		// Используем иконку из основного конфига
		iconPath = config.IconPath
		if !filepath.IsAbs(iconPath) {
			iconPath = filepath.Join(filepath.Dir(os.Args[0]), iconPath)
		}
	}

	// Проверяем существование файла иконки
	if iconPath != "" {
		if _, err := os.Stat(iconPath); os.IsNotExist(err) {
			log.Printf("Предупреждение: файл иконки не найден: %s", iconPath)

			// Ищем иконку в корне установки
			possibleIcons := []string{"icon.png", "Icon.png", "celeste.png", "Celeste.png"}
			for _, icon := range possibleIcons {
				testPath := filepath.Join(config.InstallPath, icon)
				if _, err := os.Stat(testPath); err == nil {
					iconPath = testPath
					log.Printf("Найдена альтернативная иконка: %s", iconPath)
					break
				}
			}
		}
	}

	// Формирование содержимого файла .desktop
	content := "[Desktop Entry]\n"
	content += "Type=" + config.DesktopEntry.Type + "\n"
	content += "Name=" + config.DesktopEntry.Name + "\n"
	content += "Exec=\"" + execPath + "\"\n"

	if iconPath != "" {
		content += "Icon=" + iconPath + "\n"
	}

	content += "Terminal=" + fmt.Sprintf("%t", config.DesktopEntry.Terminal) + "\n"

	if config.DesktopEntry.Categories != "" {
		content += "Categories=" + config.DesktopEntry.Categories + "\n"
	}

	if config.DesktopEntry.Comment != "" {
		content += "Comment=" + config.DesktopEntry.Comment + "\n"
	}

	// Добавляем дополнительные поля для лучшей совместимости
	content += "Version=1.0\n"
	content += "StartupNotify=true\n"
	content += "StartupWMClass=" + config.DesktopEntry.Name + "\n"

	err := ioutil.WriteFile(desktopFile, []byte(content), 0755)
	if err != nil {
		log.Printf("Ошибка при создании ярлыка: %v", err)
		widgets.QMessageBox_Warning(nil, "Предупреждение",
			"Не удалось создать ярлык в меню приложений: "+err.Error(),
			widgets.QMessageBox__Ok, widgets.QMessageBox__Ok)
	} else {
		log.Printf("Ярлык успешно создан: %s", desktopFile)

		// Сохраняем путь к файлу .desktop для деинсталлятора
		installInfo.MenuFile = desktopFile

		// Разрешаем запуск на "GNOME 3 derivatives Desktop"
		exec.Command("gio", "set", desktopFile, "metadata::trusted", "yes").Run()
		exec.Command("killall", "nautilus-desktop").Run()
		exec.Command("gio", "set", desktopFile, "metadata::trusted", "true").Run()

		// Обновляем кэш иконок и приложений
		exec.Command("gtk-update-icon-cache", "-f", "-t", filepath.Join(os.Getenv("HOME"), ".local", "share", "icons")).Run()
		exec.Command("update-desktop-database", filepath.Join(os.Getenv("HOME"), ".local", "share", "applications")).Run()
	}

	// Создаем ярлык на рабочем столе, если нужно
	desktopDir := filepath.Join(os.Getenv("HOME"), "Desktop")
	if _, err := os.Stat(desktopDir); os.IsNotExist(err) {
		// Если директория Desktop не существует, пробуем локализованное имя
		desktopDir = filepath.Join(os.Getenv("HOME"), "Рабочий стол")
	}

	if _, err := os.Stat(desktopDir); err == nil {
		desktopShortcut := filepath.Join(desktopDir, appName+".desktop")
		if err := ioutil.WriteFile(desktopShortcut, []byte(content), 0755); err != nil {
			log.Printf("Ошибка при создании ярлыка на рабочем столе: %v", err)
		} else {
			log.Printf("Ярлык на рабочем столе успешно создан: %s", desktopShortcut)

			// Сохраняем путь к файлу .desktop на рабочем столе для деинсталлятора
			installInfo.DesktopFile = desktopShortcut

			// Разрешаем запуск на "GNOME 3 derivatives Desktop"
			exec.Command("gio", "set", desktopShortcut, "metadata::trusted", "yes").Run()
			exec.Command("gio", "set", desktopShortcut, "metadata::trusted", "true").Run()
		}
	}
}

func displayError(message string) {
	widgets.QMessageBox_Critical(nil, "Ошибка", message, widgets.QMessageBox__Ok, widgets.QMessageBox__Ok)
}

func main() {
	if err := loadConfig("config.json"); err != nil {
		log.Fatal(err)
	}

	app := widgets.NewQApplication(len(os.Args), os.Args)

	// Создание темной палитры
	darkPalette := gui.NewQPalette()

	// Создаем цвета
	darkColor := gui.NewQColor3(53, 53, 53, 255)
	whiteColor := gui.NewQColor3(255, 255, 255, 255)
	darkGreyColor := gui.NewQColor3(25, 25, 25, 255)

	// Устанавливаем цвета в палитру
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

	window := widgets.NewQMainWindow(nil, 0)

	// Добавление баннера из конфигурации
	bannerLabel := widgets.NewQLabel(nil, 0)
	bannerPixmap := gui.NewQPixmap3(config.BannerPath, "", 0)
	bannerLabel.SetPixmap(bannerPixmap)
	bannerLabel.SetScaledContents(true)

	choosePathButton := widgets.NewQPushButton2("Выбрать путь", nil)
	choosePathButton.ConnectClicked(func(bool) {
		chooseInstallPath()
	})

	pathLabel = widgets.NewQLabel2("Путь установки: не выбран", nil, 0)

	// Добавляем информацию о требуемом месте
	spaceInfoLabel := widgets.NewQLabel2(fmt.Sprintf("Требуемое свободное место: %.2f ГБ", config.MinRequiredSpaceGB), nil, 0)

	// Создаем чекбокс для создания ярлыка
	createShortcutCheckBox = widgets.NewQCheckBox2("Создать ярлык запуска в меню приложений", nil)
	createShortcutCheckBox.SetChecked(true)

	// Создаем прогрессбар
	progressBar = widgets.NewQProgressBar(nil)
	progressBar.SetTextVisible(true)
	progressBar.SetAlignment(core.Qt__AlignCenter)
	progressBar.Hide() // Скрываем до начала установки

	installButton = widgets.NewQPushButton2("Начать установку", nil)
	installButton.SetEnabled(false)
	installButton.ConnectClicked(func(bool) {
		startInstallation()
	})

	// Создание вертикального layout
	layout := widgets.NewQVBoxLayout()
	layout.AddWidget(bannerLabel, 0, 0)
	layout.AddWidget(pathLabel, 0, 0)
	layout.AddWidget(spaceInfoLabel, 0, 0) // Добавляем информацию о требуемом месте
	layout.AddWidget(choosePathButton, 0, 0)
	layout.AddWidget(createShortcutCheckBox, 0, 0)
	layout.AddWidget(progressBar, 0, 0)
	layout.AddWidget(installButton, 0, 0)

	centralWidget := widgets.NewQWidget(nil, 0)
	centralWidget.SetLayout(layout)
	window.SetCentralWidget(centralWidget)

	// Устанавливаем заголовок окна с названием игры из конфига
	windowTitle := "Установщик " + config.DesktopEntry.Name
	window.SetWindowTitle(windowTitle)

	window.SetFixedSize(core.NewQSize2(500, 400))
	window.SetWindowFlags(core.Qt__Window | core.Qt__WindowTitleHint | core.Qt__WindowCloseButtonHint)
	window.Show()
	app.Exec()
}
