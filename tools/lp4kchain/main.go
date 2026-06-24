// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

func main() {
	var outputFile string
	var r io.Reader

	switch len(os.Args) {
	case 1:
		// No arguments: read from stdin
		r = os.Stdin
	case 2:
		if os.Args[1] == "-h" || os.Args[1] == "--help" {
			fmt.Fprintf(os.Stderr, "Usage: lp4kchain [lp4k-output.csv] [output.mmd|output.png]\n")
			fmt.Fprintf(os.Stderr, "\nReads lp4k CSV output and generates a Mermaid diagram of nodeclaim replacement chains.\n")
			fmt.Fprintf(os.Stderr, "If no input file is given, reads from stdin.\n")
			fmt.Fprintf(os.Stderr, "If output ends in .png, renders via mmdc (must be installed).\n")
			fmt.Fprintf(os.Stderr, "If output ends in .mmd or is omitted, writes Mermaid source to stdout or file.\n")
			os.Exit(0)
		}
		r = nil // will open file below
	default:
		r = nil
		outputFile = os.Args[2]
	}

	if r == nil {
		f, err := os.Open(os.Args[1])
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error opening %s: %v\n", os.Args[1], err)
			os.Exit(1)
		}
		defer f.Close()
		r = f
	}

	reader := csv.NewReader(r)
	header, err := reader.Read()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading CSV header: %v\n", err)
		os.Exit(1)
	}

	// Find column indices
	colIdx := make(map[string]int)
	for i, h := range header {
		name := strings.Split(h, "[")[0]
		colIdx[name] = i
	}

	requiredCols := []string{"Replacedby", "Replaces", "Instancetype", "Createdtime", "Disruptiontime"}
	for _, col := range requiredCols {
		if _, ok := colIdx[col]; !ok {
			fmt.Fprintf(os.Stderr, "Error: CSV missing required column %q\n", col)
			os.Exit(1)
		}
	}

	type nodeInfo struct {
		instanceType   string
		createdTime    string
		disruptionTime string
		replacedBy     string
		replaces       []string
	}

	nodes := make(map[string]*nodeInfo)
	var order []string

	for {
		row, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading CSV row: %v\n", err)
			continue
		}

		nc := row[0]
		info := &nodeInfo{
			instanceType:   row[colIdx["Instancetype"]],
			createdTime:    row[colIdx["Createdtime"]],
			disruptionTime: row[colIdx["Disruptiontime"]],
			replacedBy:     row[colIdx["Replacedby"]],
		}
		if rp := row[colIdx["Replaces"]]; rp != "" {
			info.replaces = strings.Split(rp, "|")
		}
		nodes[nc] = info
		order = append(order, nc)
	}

	// Build replacement chains
	chains := make(map[string]string) // nc -> replacedBy
	replacesMap := make(map[string][]string)
	for nc, info := range nodes {
		if info.replacedBy != "" {
			chains[nc] = info.replacedBy
		}
		if len(info.replaces) > 0 {
			replacesMap[nc] = info.replaces
		}
	}

	// Find chain starts (disrupted but not themselves a replacement)
	starts := make(map[string]bool)
	for nc := range chains {
		starts[nc] = true
	}
	for _, sources := range replacesMap {
		for _, src := range sources {
			// keep it as start only if it's not a replacement itself
			_ = src
		}
	}
	for nc := range replacesMap {
		delete(starts, nc)
	}

	// Build full chains from starts
	type chain struct {
		nodes []string
	}
	var allChains []chain
	for start := range starts {
		c := chain{nodes: []string{start}}
		current := start
		for {
			next, ok := chains[current]
			if !ok {
				break
			}
			c.nodes = append(c.nodes, next)
			current = next
		}
		allChains = append(allChains, c)
	}

	// Sort chains by length descending
	for i := 0; i < len(allChains); i++ {
		for j := i + 1; j < len(allChains); j++ {
			if len(allChains[j].nodes) > len(allChains[i].nodes) {
				allChains[i], allChains[j] = allChains[j], allChains[i]
			}
		}
	}

	// Generate Mermaid
	var mmd strings.Builder
	mmd.WriteString("graph LR\n")
	mmd.WriteString("    classDef disrupted fill:#ff6b6b,stroke:#c0392b,color:#fff\n")
	mmd.WriteString("    classDef intermediate fill:#ffd43b,stroke:#f08c00,color:#333\n")
	mmd.WriteString("    classDef final fill:#339af0,stroke:#1864ab,color:#fff\n")
	mmd.WriteString("\n")

	seen := make(map[string]bool)
	shownChains := 0
	maxChains := 12

	for _, c := range allChains {
		if len(c.nodes) < 3 {
			continue
		}
		if shownChains >= maxChains {
			break
		}
		// Skip if too much overlap with already shown nodes
		overlap := 0
		for _, nc := range c.nodes {
			if seen[nc] {
				overlap++
			}
		}
		if overlap > len(c.nodes)/2 {
			continue
		}

		shownChains++
		mmd.WriteString(fmt.Sprintf("    %%%% Chain %d (length %d)\n", shownChains, len(c.nodes)))

		for _, nc := range c.nodes {
			if seen[nc] {
				continue
			}
			info := nodes[nc]
			itype := info.instanceType
			if itype == "" {
				itype = "?"
			}
			ctime := ""
			if len(info.createdTime) >= 19 {
				ctime = info.createdTime[11:19]
			}
			label := fmt.Sprintf("%s<br/>%s<br/>%s", nc, itype, ctime)
			mmd.WriteString(fmt.Sprintf("    %s[\"%s\"]\n", nc, label))
			seen[nc] = true
		}

		for i := 0; i < len(c.nodes)-1; i++ {
			mmd.WriteString(fmt.Sprintf("    %s -->|replaced by| %s\n", c.nodes[i], c.nodes[i+1]))
		}
		mmd.WriteString("\n")
	}

	// Show N-to-1 consolidations (3+ sources)
	nTo1Shown := 0
	for nc, sources := range replacesMap {
		if len(sources) < 3 || seen[nc] {
			continue
		}
		if nTo1Shown >= 4 {
			break
		}
		nTo1Shown++
		mmd.WriteString(fmt.Sprintf("    %%%% N-to-1 consolidation into %s\n", nc))
		info := nodes[nc]
		itype := info.instanceType
		if itype == "" {
			itype = "?"
		}
		ctime := ""
		if len(info.createdTime) >= 19 {
			ctime = info.createdTime[11:19]
		}
		mmd.WriteString(fmt.Sprintf("    %s[\"%s<br/>%s<br/>%s\"]\n", nc, nc, itype, ctime))
		seen[nc] = true

		for _, src := range sources {
			if seen[src] {
				continue
			}
			srcInfo := nodes[src]
			sitype := ""
			sctime := ""
			if srcInfo != nil {
				sitype = srcInfo.instanceType
				if len(srcInfo.createdTime) >= 19 {
					sctime = srcInfo.createdTime[11:19]
				}
			}
			if sitype == "" {
				sitype = "?"
			}
			mmd.WriteString(fmt.Sprintf("    %s[\"%s<br/>%s<br/>%s\"]\n", src, src, sitype, sctime))
			mmd.WriteString(fmt.Sprintf("    %s -->|consolidated| %s\n", src, nc))
			seen[src] = true
		}
		mmd.WriteString("\n")
	}

	// Apply styles
	mmd.WriteString("    %% Styles\n")
	for _, c := range allChains {
		for i, nc := range c.nodes {
			if !seen[nc] {
				continue
			}
			if i == 0 {
				mmd.WriteString(fmt.Sprintf("    class %s disrupted\n", nc))
			} else if i == len(c.nodes)-1 {
				mmd.WriteString(fmt.Sprintf("    class %s final\n", nc))
			} else {
				mmd.WriteString(fmt.Sprintf("    class %s intermediate\n", nc))
			}
		}
	}

	mermaidContent := mmd.String()

	// Output
	if outputFile == "" || strings.HasSuffix(outputFile, ".mmd") {
		if outputFile == "" {
			fmt.Print(mermaidContent)
		} else {
			if err := os.WriteFile(outputFile, []byte(mermaidContent), 0644); err != nil {
				fmt.Fprintf(os.Stderr, "Error writing %s: %v\n", outputFile, err)
				os.Exit(1)
			}
			fmt.Fprintf(os.Stderr, "Written Mermaid source to %s\n", outputFile)
		}
	} else if strings.HasSuffix(outputFile, ".png") || strings.HasSuffix(outputFile, ".svg") || strings.HasSuffix(outputFile, ".pdf") {
		// Write temp .mmd file, then invoke mmdc
		mmdPath := strings.TrimSuffix(outputFile, ".png")
		mmdPath = strings.TrimSuffix(mmdPath, ".svg")
		mmdPath = strings.TrimSuffix(mmdPath, ".pdf")
		mmdPath += ".mmd"

		if err := os.WriteFile(mmdPath, []byte(mermaidContent), 0644); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing %s: %v\n", mmdPath, err)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "Written Mermaid source to %s\n", mmdPath)

		// Check if mmdc is available
		mmdcPath, err := exec.LookPath("mmdc")
		if err != nil {
			fmt.Fprintf(os.Stderr, "mmdc not found in PATH. Install with: npm install -g @mermaid-js/mermaid-cli\n")
			fmt.Fprintf(os.Stderr, "Mermaid source saved to %s — render manually or use mermaid.live\n", mmdPath)
			os.Exit(1)
		}

		cmd := exec.Command(mmdcPath, "-i", mmdPath, "-o", outputFile, "-w", "2400", "-H", "1600", "--backgroundColor", "white")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "Error running mmdc: %v\n", err)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "Rendered %s\n", outputFile)
	}
}
