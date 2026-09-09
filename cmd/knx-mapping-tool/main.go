package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"knx-tuya-gw/internal/config"
	"knx-tuya-gw/internal/knxreview"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "import":
		runImport(os.Args[2:])
	case "finalize":
		runFinalize(os.Args[2:])
	case "validate":
		runValidate(os.Args[2:])
	default:
		usage()
		os.Exit(2)
	}
}

func runImport(arguments []string) {
	flags := flag.NewFlagSet("import", flag.ExitOnError)
	input := flags.String("input", "", "source KNX .xlsx file")
	review := flags.String("review", "data/knx-mapping-review.csv", "review CSV output")
	base := flags.String("base", "config/knx-mapping.json", "confirmed base bundle")
	configPath := flags.String("config", "config/config.json", "runtime config for DP capacities")
	_ = flags.Parse(arguments)
	if *input == "" {
		fatalf("-input is required")
	}
	cfg := loadConfig(*configPath)
	counts, err := knxreview.ImportXLSX(
		*input,
		*review,
		*base,
		cfg.TuyaMQTT.GatewayDPPool.Capacities,
	)
	if err != nil {
		fatalf("import failed: %v", err)
	}
	fmt.Printf("review generated: %s (%s)\n", *review, strings.Join(knxreview.SortedCounts(counts), ", "))
}

func runFinalize(arguments []string) {
	flags := flag.NewFlagSet("finalize", flag.ExitOnError)
	review := flags.String("review", "data/knx-mapping-review.csv", "review CSV input")
	base := flags.String("base", "config/knx-mapping.json", "confirmed base bundle")
	output := flags.String(
		"output",
		"data/knx-mapping-for-upgrade.json",
		"validated upgrade bundle output",
	)
	version := flags.String("version", "", "runtime bundle version")
	configPath := flags.String("config", "config/config.json", "runtime config for DP validation")
	_ = flags.Parse(arguments)
	cfg := loadConfig(*configPath)
	count, err := knxreview.Finalize(
		*review,
		*base,
		*output,
		*version,
		cfg.TuyaMQTT.GatewayDPPool,
	)
	if err != nil {
		fatalf("finalize failed: %v", err)
	}
	fmt.Printf("upgrade bundle generated: %s (mappings=%d)\n", *output, count)
}

func runValidate(arguments []string) {
	flags := flag.NewFlagSet("validate", flag.ExitOnError)
	input := flags.String("input", "data/knx-mapping-for-upgrade.json", "bundle to validate")
	configPath := flags.String("config", "config/config.json", "runtime config for DP validation")
	_ = flags.Parse(arguments)
	cfg := loadConfig(*configPath)
	count, err := knxreview.ValidateBundle(*input, cfg.TuyaMQTT.GatewayDPPool)
	if err != nil {
		fatalf("validation failed: %v", err)
	}
	fmt.Printf("bundle valid: %s (mappings=%d)\n", *input, count)
}

func loadConfig(path string) *config.Config {
	cfg, err := config.Load(path)
	if err != nil {
		fatalf("load config failed: %v", err)
	}
	return cfg
}

func fatalf(format string, values ...interface{}) {
	fmt.Fprintf(os.Stderr, format+"\n", values...)
	os.Exit(1)
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: knx-mapping-tool <import|finalize|validate> [flags]")
}
