package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"

	"github.com/spf13/cobra"
)

func newWatchCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "watch",
		Short: "Watch all running workers, alert on events needing attention",
		RunE:  runWatch,
	}
}

type workerStream struct {
	name      string
	container string
	cancel    func()
}

func runWatch(cmd *cobra.Command, args []string) error {
	projectRoot, err := gitRoot()
	if err != nil {
		return fmt.Errorf("not in a git repo: %w", err)
	}

	// Find running containers
	out, err := exec.Command("podman", "ps",
		"--filter", "label=sindri.project="+projectRoot,
		"--format", "json",
	).Output()
	if err != nil {
		return fmt.Errorf("podman ps: %w", err)
	}

	var containers []podmanContainer
	if len(strings.TrimSpace(string(out))) > 0 {
		if err := json.Unmarshal(out, &containers); err != nil {
			return fmt.Errorf("parse: %w", err)
		}
	}

	running := 0
	for _, c := range containers {
		if c.State == "running" {
			running++
		}
	}

	if running == 0 {
		fmt.Println("No running workers.")
		return nil
	}

	fmt.Fprintf(os.Stderr, "Watching %d worker(s)... (ctrl-c to stop)\n\n", running)

	var wg sync.WaitGroup
	for _, c := range containers {
		if c.State != "running" {
			continue
		}
		worker := c.Labels["sindri.worker"]
		cName := ""
		if len(c.Names) > 0 {
			cName = c.Names[0]
		}
		if worker == "" || cName == "" {
			continue
		}

		wg.Add(1)
		go func(worker, cName string) {
			defer wg.Done()
			watchWorker(worker, cName)
		}(worker, cName)
	}

	wg.Wait()
	return nil
}

func watchWorker(worker, containerName string) {
	icon := "🔨"
	if worker == "_reviewer" {
		icon = "👑"
		worker = "reviewer"
	}

	logs := exec.Command("podman", "logs", "-f", "--tail", "0", containerName)
	stdout, err := logs.StdoutPipe()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s %s: failed to connect: %v\n", icon, worker, err)
		return
	}
	logs.Stderr = logs.Stdout

	if err := logs.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "%s %s: failed to start logs: %v\n", icon, worker, err)
		return
	}

	prefix := fmt.Sprintf("%s %s", icon, worker)

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()

		// Non-JSON lines
		if !strings.HasPrefix(line, "{") {
			if line != "" {
				fmt.Printf("%s%s %s%s\n", dim, prefix, line, reset)
			}
			continue
		}

		var event map[string]interface{}
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			continue
		}

		formatWatchEvent(prefix, event)
	}

	fmt.Printf("%s%s [stream ended]%s\n", dim, prefix, reset)
	_ = logs.Wait()
}

func formatWatchEvent(prefix string, event map[string]interface{}) {
	typ, _ := event["type"].(string)

	switch typ {
	case "assistant":
		msg, ok := event["message"].(map[string]interface{})
		if !ok {
			return
		}
		content, ok := msg["content"].([]interface{})
		if !ok {
			return
		}
		for _, c := range content {
			block, ok := c.(map[string]interface{})
			if !ok {
				continue
			}
			blockType, _ := block["type"].(string)
			switch blockType {
			case "text":
				text, _ := block["text"].(string)
				fmt.Printf("\n%s %s🤖💬 %s%s\n", prefix, white, text, reset)
			case "tool_use":
				name, _ := block["name"].(string)

				// Alert on AskUserQuestion
				if name == "AskUserQuestion" {
					input, _ := block["input"].(map[string]interface{})
					questions, _ := input["questions"].([]interface{})
					questionText := ""
					if len(questions) > 0 {
						q, _ := questions[0].(map[string]interface{})
						questionText, _ = q["question"].(string)
					}
					fmt.Printf("\n%s %s⚠ NEEDS INPUT: %s%s\n", prefix, yellow, questionText, reset)
					fmt.Printf("%s   → sindri worker input %s \"your answer\"\n", prefix, strings.TrimPrefix(prefix, "🔨 "))
					return
				}

				input, _ := block["input"].(map[string]interface{})
				switch name {
				case "Bash":
					cmd, _ := input["command"].(string)
					if len(cmd) > 100 {
						cmd = cmd[:100] + "..."
					}
					fmt.Printf("%s %s🤖🔨 %s %s%s\n", prefix, dim, name, cmd, reset)
				case "Read", "Edit", "Write":
					path, _ := input["file_path"].(string)
					fmt.Printf("%s %s🤖🔨 %s %s%s\n", prefix, dim, name, path, reset)
				default:
					fmt.Printf("%s %s🤖🔨 %s%s\n", prefix, dim, name, reset)
				}
			}
		}

	case "user":
		toolResult, ok := event["tool_use_result"].(map[string]interface{})
		if !ok {
			return
		}
		if stdout, ok := toolResult["stdout"].(string); ok {
			out := stdout
			if len(out) > 150 {
				out = out[:150] + "..."
			}
			fmt.Printf("%s %s   ← %s%s\n", prefix, dim, out, reset)
		}

	case "result":
		result, _ := event["result"].(string)
		cost, _ := event["total_cost_usd"].(float64)
		if len(result) > 150 {
			result = result[:150] + "..."
		}
		fmt.Printf("\n%s %s▸ Done ($%.4f)%s\n", prefix, green, cost, reset)
		if result != "" {
			fmt.Printf("%s %s🤖💬 %s%s\n\n", prefix, white, result, reset)
		}
	}
}
