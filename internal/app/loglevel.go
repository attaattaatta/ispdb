package app

func logLevelRank(level string) int {
	switch level {
	case "debug":
		return 5
	case "info":
		return 4
	case "warn":
		return 3
	case "error":
		return 2
	case "crit":
		return 1
	default:
		return 0
	}
}

func consoleLevelEnabled(current string, target string) bool {
	return logLevelRank(current) >= logLevelRank(target)
}
