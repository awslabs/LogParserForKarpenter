// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

func main() {
	minLength := flag.Int("min-length", 3, "minimum chain length to display")
	maxChains := flag.Int("max-chains", 12, "maximum number of chains to display")
	showAll := flag.Bool("all", false, "show all replacement chains (overrides --min-length and --max-chains)")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: lp4kchain [flags] [lp4k-output.csv] [output.mmd|output.png]\n")
		fmt.Fprintf(os.Stderr, "\nReads lp4k CSV output and generates a Mermaid diagram of nodeclaim replacement chains.\n")
		fmt.Fprintf(os.Stderr, "If no input file is given, reads from stdin.\n")
		fmt.Fprintf(os.Stderr, "If output ends in .png, renders via mmdc (must be installed).\n")
		fmt.Fprintf(os.Stderr, "If output ends in .mmd or is omitted, writes Mermaid source to stdout or file.\n")
		fmt.Fprintf(os.Stderr, "\nFlags:\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	if *showAll {
		*minLength = 1
		*maxChains = 0
	}

	// Remaining positional args after flags
	args := flag.Args()
	var outputFile string
	var r io.Reader

	switch len(args) {
	case 0:
		r = os.Stdin
	case 1:
		r = nil
	default:
		r = nil
		outputFile = args[1]
	}

	if r == nil {
		f, err := os.Open(args[0])
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error opening %s: %v\n", args[0], err)
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
	seenEdges := make(map[string]bool)
	shownChains := 0

	for _, c := range allChains {
		if len(c.nodes) < *minLength {
			continue
		}
		if *maxChains > 0 && shownChains >= *maxChains {
			break
		}
		// Skip if too much overlap with already shown nodes (unless --all)
		if !*showAll {
			overlap := 0
			for _, nc := range c.nodes {
				if seen[nc] {
					overlap++
				}
			}
			if overlap > len(c.nodes)/2 {
				continue
			}
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
			edgeKey := c.nodes[i] + "->" + c.nodes[i+1]
			if !seenEdges[edgeKey] {
				mmd.WriteString(fmt.Sprintf("    %s -->|replaced by| %s\n", c.nodes[i], c.nodes[i+1]))
				seenEdges[edgeKey] = true
			}
		}
		mmd.WriteString("\n")
	}

	// Show N-to-1 consolidations (only genuine multi-source merges not already shown in chains)
	nTo1Limit := 4
	if *showAll {
		nTo1Limit = 0
	}
	nTo1Shown := 0
	for nc, sources := range replacesMap {
		if len(sources) < 2 {
			continue
		}
		if !*showAll && len(sources) < 3 {
			continue
		}
		// Skip if all nodes involved are already displayed in chains
		if seen[nc] {
			allSourcesSeen := true
			for _, src := range sources {
				if !seen[src] {
					allSourcesSeen = false
					break
				}
			}
			if allSourcesSeen {
				continue
			}
		}
		if nTo1Limit > 0 && nTo1Shown >= nTo1Limit {
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
		if !seen[nc] {
			mmd.WriteString(fmt.Sprintf("    %s[\"%s<br/>%s<br/>%s\"]\n", nc, nc, itype, ctime))
			seen[nc] = true
		}

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
			edgeKey := src + "->" + nc
			if !seenEdges[edgeKey] {
				mmd.WriteString(fmt.Sprintf("    %s -->|consolidated| %s\n", src, nc))
				seenEdges[edgeKey] = true
			}
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

	// Statistics
	totalReplacements := len(chains)
	oneToOne := 0
	nToOne := 0
	for _, sources := range replacesMap {
		if len(sources) == 1 {
			oneToOne++
		} else {
			nToOne++
		}
	}
	totalChains := len(allChains)
	longestChain := 0
	if len(allChains) > 0 {
		longestChain = len(allChains[0].nodes)
	}
	displayedEdges := 0
	for _, c := range allChains {
		for _, nc := range c.nodes {
			if seen[nc] {
				displayedEdges++
			}
		}
	}

	// Legend and statistics subgraph
	mmd.WriteString("\n    %% Legend and Statistics\n")
	mmd.WriteString("    subgraph Legend\n")
	mmd.WriteString("        direction LR\n")
	mmd.WriteString("        L1[\"Disrupted\\n(chain start)\"]:::disrupted\n")
	mmd.WriteString("        L2[\"Intermediate\\n(churn)\"]:::intermediate\n")
	mmd.WriteString("        L3[\"Final\\n(chain end)\"]:::final\n")
	mmd.WriteString("        L1 ~~~ L2 ~~~ L3\n")
	mmd.WriteString("    end\n")
	mmd.WriteString("    subgraph Statistics\n")
	mmd.WriteString("        direction LR\n")
	mmd.WriteString(fmt.Sprintf("        S1[\"Total replacements: %d\"]\n", totalReplacements))
	mmd.WriteString(fmt.Sprintf("        S2[\"1-to-1: %d | N-to-1: %d\"]\n", oneToOne, nToOne))
	mmd.WriteString(fmt.Sprintf("        S3[\"Chains: %d | Longest: %d nodes\"]\n", totalChains, longestChain))
	mmd.WriteString(fmt.Sprintf("        S4[\"Displayed: %d of %d nodeclaims\"]\n", len(seen), len(nodes)))
	mmd.WriteString("        S1 ~~~ S2 ~~~ S3 ~~~ S4\n")
	mmd.WriteString("    end\n")

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
		// Write .mmd file, then invoke mmdc
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
