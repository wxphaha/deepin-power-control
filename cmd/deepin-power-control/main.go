package main

import (
	"fmt"
	"os"
	"strings"

	"deepin-power-control/internal/config"
	"deepin-power-control/internal/debugger"
	"deepin-power-control/internal/global"
	"deepin-power-control/internal/monitor"
	"deepin-power-control/internal/utils"

	"github.com/spf13/cobra"
)

var (
	debugMode   bool
	verboseMode bool
	configPath  string
	logToFile   bool

	debugTarget string
	debugExpect string
)

func checkRoot() error {
	if os.Geteuid() != 0 {
		return fmt.Errorf("this command must be run as root (UID 0)")
	}
	return nil
}

var rootCmd = &cobra.Command{
	Use:           "deepin-power-control",
	Short:         "Deepin System Power Control tool",
	Long:          `A command line tool to manage power modes and WiFi adaptive power saving for Deepin/UOS.`,
	SilenceUsage:  true,
	SilenceErrors: true,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		isDebugSubCmd := strings.Contains(cmd.CommandPath(), " debug")
		if isDebugSubCmd && !cmd.Flags().Changed("logtofile") {
			logToFile = true
		}

		global.InitLog(verboseMode, logToFile)
		global.LoadConfig(configPath)
		global.SetLogLevel(global.AppConfig.LogLevel)
		if debugMode {
			global.SetLogLevel(global.LevelDebug)
		}
		global.Debug("Debug mode enabled")
	},
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

var setCmd = &cobra.Command{
	Use:   "set [mode]",
	Short: "Set power mode",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return fmt.Errorf("missing mode! Valid modes are: %s", strings.Join(config.ValidModes, ", "))
		}
		if !config.IsValidMode(args[0]) {
			return fmt.Errorf("invalid mode %q! Valid modes are: %s", args[0], strings.Join(config.ValidModes, ", "))
		}
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := checkRoot(); err != nil {
			return err
		}
		if err := utils.LockTlp(); err != nil {
			return err
		}
		defer utils.UnlockTlp()

		mode := args[0]
		global.Info("Applying power mode: %s", mode)
		if err := config.ApplyMode(mode); err != nil {
			return fmt.Errorf("failed to apply mode: %w", err)
		}
		return config.InitStart()
	},
}

var initStartCmd = &cobra.Command{
	Use:   "init-start",
	Short: "Start TLP service without changing config",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := checkRoot(); err != nil {
			return err
		}
		if err := utils.LockTlp(); err != nil {
			return err
		}
		defer utils.UnlockTlp()

		global.Info("Initializing TLP service...")
		return config.InitStart()
	},
}

var wifiCmd = &cobra.Command{
	Use:   "wifi [on|off]",
	Short: "Manage WiFi idle monitor daemon",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return fmt.Errorf("missing state! Valid state are: on, off")
		}
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := checkRoot(); err != nil {
			return err
		}

		action := args[0]
		switch action {
		case "on":
			global.Info("Starting WiFi idle daemon...")
			return monitor.RunWifiEngine()
		case "off":
			global.Info("Stopping WiFi idle daemon...")
			return monitor.StopWifiEngine()
		default:
			return fmt.Errorf("invalid action: %s. Use 'on' or 'off'", action)
		}
	},
}

var idleCmd = &cobra.Command{
	Use:   "idle",
	Short: "Manage idle power saving features",
}

var debugCmd = &cobra.Command{
	Use:   "debug",
	Short: "Automated configuration debugger for TLP conflicts",
	Long:  `Binary search and minimize TLP configuration issues across reboots.`,
}

var debugInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize a new debug session",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := checkRoot(); err != nil {
			return err
		}
		if debugTarget == "" || debugExpect == "" {
			return fmt.Errorf("--target and --expect are required")
		}

		if _, err := os.Stat(global.DebugLogPath); err == nil {
			_ = os.Truncate(global.DebugLogPath, 0)
		}

		probe := &debugger.SysfsProbe{
			Path:   debugTarget,
			Expect: debugExpect,
		}
		engine := debugger.NewEngine(probe)

		return engine.Init(debugTarget, debugExpect)
	},
}

var debugResumeCmd = &cobra.Command{
	Use:   "resume",
	Short: "Resume an existing debug session (usually called by systemd)",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := checkRoot(); err != nil {
			return err
		}

		engine := debugger.NewEngine(nil)

		return engine.Resume()
	},
}

var debugAbortCmd = &cobra.Command{
	Use:   "abort",
	Short: "Abort an existing debug session",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := checkRoot(); err != nil {
			return err
		}

		engine := debugger.NewEngine(nil)

		engine.Abort()
		return nil
	},
}

func init() {
	rootCmd.PersistentFlags().BoolVarP(&debugMode, "debug", "d", false, "Enable debug logging")
	rootCmd.PersistentFlags().BoolVarP(&verboseMode, "verbose", "v", false, "Show log in console")
	rootCmd.PersistentFlags().StringVarP(&configPath, "config", "c", "/etc/deepin-power-control/config.yaml", "Path to config file")
	rootCmd.PersistentFlags().BoolVar(&logToFile, "logtofile", false, "Log to file instead of syslog")

	debugCmd.PersistentFlags().StringVar(&debugTarget, "target", "", "Path to sysfs file (e.g. /sys/bus/pci/...)")
	debugCmd.PersistentFlags().StringVar(&debugExpect, "expect", "", "Expected value in the target file (e.g. 'auto')")

	rootCmd.AddCommand(setCmd, initStartCmd, debugCmd)
	debugCmd.AddCommand(debugInitCmd, debugResumeCmd, debugAbortCmd)

	idleCmd.AddCommand(wifiCmd)
	rootCmd.AddCommand(idleCmd)

	//rootCmd.CompletionOptions.DisableDefaultCmd = true
	rootCmd.AddCommand(&cobra.Command{
		Use:    "completion",
		Hidden: true,
		// Hide the parent persistent pre/post run
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
		PersistentPostRunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			err := cmd.Root().GenBashCompletion(os.Stdout)
			if err != nil {
				return fmt.Errorf("failed to generate bash completion: %w", err)
			}
			return nil
		},
	})

}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		global.Error("%v", err)
		os.Exit(1)
	}
}
