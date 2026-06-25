// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"
)

type poolAZStats struct {
	total          int
	interrupted    int
	underutilized  int
	empty          int
}

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: lp4kstats [flags] [lp4k-output.csv] [output.md]\n")
		fmt.Fprintf(os.Stderr, "\nReads lp4k CSV output and generates a Markdown statistics report with Mermaid charts.\n")
		fmt.Fprintf(os.Stderr, "If no input file is given, reads from stdin.\n")
		fmt.Fprintf(os.Stderr, "If no output file is given, writes to stdout.\n")
		fmt.Fprintf(os.Stderr, "\nFlags:\n")
		flag.PrintDefaults()
	}
	flag.Parse()

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

	colIdx := make(map[string]int)
	for i, h := range header {
		name := strings.Split(h, "[")[0]
		colIdx[name] = i
	}

	requiredCols := []string{"Nodeclaim", "Nodepool", "Zone", "Capacitytype", "Instancetype", "Interruptionkind", "Disruptionreason", "Interruptiontime", "Disruptiontime"}
	for _, col := range requiredCols {
		if _, ok := colIdx[col]; !ok {
			fmt.Fprintf(os.Stderr, "Error: CSV missing required column %q\n", col)
			os.Exit(1)
		}
	}

	type nodeclaimRecord struct {
		name             string
		nodepool         string
		zone             string
		capacityType     string
		instanceType     string
		interruptKind    string
		disruptionReason string
		interruptionTime string
		disruptionTime   string
	}

	var records []nodeclaimRecord

	for {
		row, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading CSV row: %v\n", err)
			continue
		}

		records = append(records, nodeclaimRecord{
			name:             row[colIdx["Nodeclaim"]],
			nodepool:         row[colIdx["Nodepool"]],
			zone:             row[colIdx["Zone"]],
			capacityType:     row[colIdx["Capacitytype"]],
			instanceType:     row[colIdx["Instancetype"]],
			interruptKind:    row[colIdx["Interruptionkind"]],
			disruptionReason: row[colIdx["Disruptionreason"]],
			interruptionTime: row[colIdx["Interruptiontime"]],
			disruptionTime:   row[colIdx["Disruptiontime"]],
		})
	}

	if len(records) == 0 {
		fmt.Fprintf(os.Stderr, "No records found in CSV\n")
		os.Exit(1)
	}

	// Derive region from zones
	regions := make(map[string]bool)
	for _, rec := range records {
		if rec.zone != "" {
			region := rec.zone[:len(rec.zone)-1]
			regions[region] = true
		}
	}

	// Count records without a zone
	noZoneCount := 0
	for _, rec := range records {
		if rec.zone == "" {
			noZoneCount++
		}
	}

	// Collect stats
	// region -> stats
	regionTotal := make(map[string]int)
	regionInterrupted := make(map[string]int)
	regionUnderutilized := make(map[string]int)
	regionEmpty := make(map[string]int)
	// region -> AZ -> stats
	regionAZ := make(map[string]map[string]*poolAZStats)
	// region -> pool -> stats
	regionPool := make(map[string]map[string]*poolAZStats)
	// region -> pool -> AZ -> stats
	regionPoolAZ := make(map[string]map[string]map[string]*poolAZStats)

	for _, rec := range records {
		if rec.zone == "" {
			continue
		}
		region := rec.zone[:len(rec.zone)-1]
		az := rec.zone
		pool := rec.nodepool
		interrupted := rec.interruptKind == "spot_interrupted"
		underutilized := rec.disruptionReason == "underutilized"
		empty := rec.disruptionReason == "empty"

		regionTotal[region]++
		if interrupted {
			regionInterrupted[region]++
		}
		if underutilized {
			regionUnderutilized[region]++
		}
		if empty {
			regionEmpty[region]++
		}

		// Region -> AZ
		if regionAZ[region] == nil {
			regionAZ[region] = make(map[string]*poolAZStats)
		}
		if regionAZ[region][az] == nil {
			regionAZ[region][az] = &poolAZStats{}
		}
		regionAZ[region][az].total++
		if interrupted {
			regionAZ[region][az].interrupted++
		}
		if underutilized {
			regionAZ[region][az].underutilized++
		}
		if empty {
			regionAZ[region][az].empty++
		}

		// Region -> Pool
		if regionPool[region] == nil {
			regionPool[region] = make(map[string]*poolAZStats)
		}
		if regionPool[region][pool] == nil {
			regionPool[region][pool] = &poolAZStats{}
		}
		regionPool[region][pool].total++
		if interrupted {
			regionPool[region][pool].interrupted++
		}
		if underutilized {
			regionPool[region][pool].underutilized++
		}
		if empty {
			regionPool[region][pool].empty++
		}

		// Region -> Pool -> AZ
		if regionPoolAZ[region] == nil {
			regionPoolAZ[region] = make(map[string]map[string]*poolAZStats)
		}
		if regionPoolAZ[region][pool] == nil {
			regionPoolAZ[region][pool] = make(map[string]*poolAZStats)
		}
		if regionPoolAZ[region][pool][az] == nil {
			regionPoolAZ[region][pool][az] = &poolAZStats{}
		}
		regionPoolAZ[region][pool][az].total++
		if interrupted {
			regionPoolAZ[region][pool][az].interrupted++
		}
		if underutilized {
			regionPoolAZ[region][pool][az].underutilized++
		}
		if empty {
			regionPoolAZ[region][pool][az].empty++
		}
	}

	// Sort regions
	sortedRegions := make([]string, 0, len(regions))
	for r := range regions {
		sortedRegions = append(sortedRegions, r)
	}
	sort.Strings(sortedRegions)

	// Generate markdown
	var md strings.Builder

	md.WriteString("# Karpenter Statistics Report\n\n")

	for _, region := range sortedRegions {
		md.WriteString(fmt.Sprintf("## Region: %s\n\n", region))
		md.WriteString(fmt.Sprintf("| Metric | Value |\n"))
		md.WriteString(fmt.Sprintf("|--------|-------|\n"))
		md.WriteString(fmt.Sprintf("| Total NodeClaims | %d |\n", regionTotal[region]))
		interruptPct := 0.0
		if regionTotal[region] > 0 {
			interruptPct = float64(regionInterrupted[region]) / float64(regionTotal[region]) * 100
		}
		md.WriteString(fmt.Sprintf("| Spot Interruptions (Rate) | %d (%.1f%%) |\n", regionInterrupted[region], interruptPct))
		underutilPct := 0.0
		if regionTotal[region] > 0 {
			underutilPct = float64(regionUnderutilized[region]) / float64(regionTotal[region]) * 100
		}
		md.WriteString(fmt.Sprintf("| Underutilized Disruptions (Rate) | %d (%.1f%%) |\n", regionUnderutilized[region], underutilPct))
		emptyPct := 0.0
		if regionTotal[region] > 0 {
			emptyPct = float64(regionEmpty[region]) / float64(regionTotal[region]) * 100
		}
		md.WriteString(fmt.Sprintf("| Empty Disruptions (Rate) | %d (%.1f%%) |\n", regionEmpty[region], emptyPct))
		md.WriteString("\n")

		if noZoneCount > 0 {
			md.WriteString(fmt.Sprintf("*Note: %d NodeClaim(s) without an AZ (never launched) excluded from statistics.*\n\n", noZoneCount))
		}

		azStats := regionAZ[region]
		sortedAZs := sortedKeys(azStats)
		poolStats := regionPool[region]
		sortedPools := sortedKeys(poolStats)

		md.WriteString("### Region Overview\n\n")

		// Row 1: NodeClaims by AZ + NodeClaims by NodePool
		md.WriteString("<!-- charts-row -->\n\n")

		md.WriteString("```mermaid\n")
		md.WriteString("pie title NodeClaims by AZ\n")
		for _, az := range sortedAZs {
			md.WriteString(fmt.Sprintf("    \"%s\" : %d\n", az, azStats[az].total))
		}
		md.WriteString("```\n\n")

		md.WriteString("```mermaid\n")
		md.WriteString("pie title NodeClaims by NodePool\n")
		for _, pool := range sortedPools {
			md.WriteString(fmt.Sprintf("    \"%s\" : %d\n", pool, poolStats[pool].total))
		}
		md.WriteString("```\n\n")

		md.WriteString("<!-- /charts-row -->\n\n")

		// Row 2: Spot Interruptions by AZ + Underutilized by AZ
		md.WriteString("<!-- charts-row -->\n\n")

		if regionInterrupted[region] > 0 {
			md.WriteString("```mermaid\n")
			md.WriteString("pie title Spot Interruptions by AZ\n")
			for _, az := range sortedAZs {
				if azStats[az].interrupted > 0 {
					md.WriteString(fmt.Sprintf("    \"%s\" : %d\n", az, azStats[az].interrupted))
				}
			}
			md.WriteString("```\n\n")
		}

		if regionUnderutilized[region] > 0 {
			md.WriteString("```mermaid\n")
			md.WriteString("pie title Underutilized Disruptions by AZ\n")
			for _, az := range sortedAZs {
				if azStats[az].underutilized > 0 {
					md.WriteString(fmt.Sprintf("    \"%s\" : %d\n", az, azStats[az].underutilized))
				}
			}
			md.WriteString("```\n\n")
		}

		if regionEmpty[region] > 0 {
			md.WriteString("```mermaid\n")
			md.WriteString("pie title Empty Disruptions by AZ\n")
			for _, az := range sortedAZs {
				if azStats[az].empty > 0 {
					md.WriteString(fmt.Sprintf("    \"%s\" : %d\n", az, azStats[az].empty))
				}
			}
			md.WriteString("```\n\n")
		}

		md.WriteString("<!-- /charts-row -->\n\n")

		// Per-AZ breakdown
		md.WriteString("### Per Availability Zone Overview\n\n")
		md.WriteString("| AZ | Total | Spot Interruptions (Rate) | Underutilized Disruptions (Rate) | Empty Disruptions (Rate) |\n")
		md.WriteString("|----|-------|------------------------|---------------------|-------------|\n")
		for _, az := range sortedAZs {
			s := azStats[az]
			azIntPct := 0.0
			azUtilPct := 0.0
			azEmptyPct := 0.0
			if s.total > 0 {
				azIntPct = float64(s.interrupted) / float64(s.total) * 100
				azUtilPct = float64(s.underutilized) / float64(s.total) * 100
				azEmptyPct = float64(s.empty) / float64(s.total) * 100
			}
			md.WriteString(fmt.Sprintf("| %s | %d | %d (%.1f%%) | %d (%.1f%%) | %d (%.1f%%) |\n", az, s.total, s.interrupted, azIntPct, s.underutilized, azUtilPct, s.empty, azEmptyPct))
		}
		md.WriteString("\n")

		// Per-NodePool drill-down
		for _, pool := range sortedPools {
			ps := poolStats[pool]
			md.WriteString(fmt.Sprintf("### NodePool: %s\n\n", pool))

			// Two tables side by side
			md.WriteString("<!-- tables-row -->\n\n")

			// General table
			md.WriteString(fmt.Sprintf("#### %s - General\n\n", pool))
			md.WriteString(fmt.Sprintf("| Metric | Value |\n"))
			md.WriteString(fmt.Sprintf("|--------|-------|\n"))
			md.WriteString(fmt.Sprintf("| Total NodeClaims | %d |\n", ps.total))
			poolInterruptPct := 0.0
			if ps.total > 0 {
				poolInterruptPct = float64(ps.interrupted) / float64(ps.total) * 100
			}
			md.WriteString(fmt.Sprintf("| Spot Interruptions (Rate) | %d (%.1f%%) |\n", ps.interrupted, poolInterruptPct))
			poolUnderutilPct := 0.0
			if ps.total > 0 {
				poolUnderutilPct = float64(ps.underutilized) / float64(ps.total) * 100
			}
			md.WriteString(fmt.Sprintf("| Underutilized Disruptions (Rate) | %d (%.1f%%) |\n", ps.underutilized, poolUnderutilPct))
			poolEmptyPct := 0.0
			if ps.total > 0 {
				poolEmptyPct = float64(ps.empty) / float64(ps.total) * 100
			}
			md.WriteString(fmt.Sprintf("| Empty Disruptions (Rate) | %d (%.1f%%) |\n", ps.empty, poolEmptyPct))
			md.WriteString("\n")

			// By AZ table
			poolAZs := regionPoolAZ[region][pool]
			sortedPoolAZs := sortedKeys(poolAZs)

			md.WriteString(fmt.Sprintf("#### %s — by Availability Zone\n\n", pool))
			md.WriteString("| AZ | Total | Spot Interruptions (Rate) | Underutilized Disruptions (Rate) | Empty Disruptions (Rate) |\n")
			md.WriteString("|----|-------|------------------------|---------------------|-------------|\n")
			for _, az := range sortedPoolAZs {
				s := poolAZs[az]
				azIntPct := 0.0
				azUtilPct := 0.0
				azEmptyPct := 0.0
				if s.total > 0 {
					azIntPct = float64(s.interrupted) / float64(s.total) * 100
					azUtilPct = float64(s.underutilized) / float64(s.total) * 100
					azEmptyPct = float64(s.empty) / float64(s.total) * 100
				}
				md.WriteString(fmt.Sprintf("| %s | %d | %d (%.1f%%) | %d (%.1f%%) | %d (%.1f%%) |\n", az, s.total, s.interrupted, azIntPct, s.underutilized, azUtilPct, s.empty, azEmptyPct))
			}
			md.WriteString("\n")

			md.WriteString("<!-- /tables-row -->\n\n")

			// Pie charts row for this pool
			md.WriteString("<!-- charts-row -->\n\n")

			md.WriteString("```mermaid\n")
			md.WriteString(fmt.Sprintf("pie title %s - NodeClaims by AZ\n", pool))
			for _, az := range sortedPoolAZs {
				md.WriteString(fmt.Sprintf("    \"%s\" : %d\n", az, poolAZs[az].total))
			}
			md.WriteString("```\n\n")

			if ps.interrupted > 0 {
				md.WriteString("```mermaid\n")
				md.WriteString(fmt.Sprintf("pie title %s - Spot Interruptions by AZ\n", pool))
				for _, az := range sortedPoolAZs {
					if poolAZs[az].interrupted > 0 {
						md.WriteString(fmt.Sprintf("    \"%s\" : %d\n", az, poolAZs[az].interrupted))
					}
				}
				md.WriteString("```\n\n")
			} else {
				md.WriteString("<!-- placeholder: No Spot Interruptions -->\n\n")
			}

			if ps.underutilized > 0 {
				md.WriteString("```mermaid\n")
				md.WriteString(fmt.Sprintf("pie title %s - Underutilized Disruptions by AZ\n", pool))
				for _, az := range sortedPoolAZs {
					if poolAZs[az].underutilized > 0 {
						md.WriteString(fmt.Sprintf("    \"%s\" : %d\n", az, poolAZs[az].underutilized))
					}
				}
				md.WriteString("```\n\n")
			} else {
				md.WriteString("<!-- placeholder: No Underutilized Disruptions -->\n\n")
			}

			if ps.empty > 0 {
				md.WriteString("```mermaid\n")
				md.WriteString(fmt.Sprintf("pie title %s - Empty Disruptions by AZ\n", pool))
				for _, az := range sortedPoolAZs {
					if poolAZs[az].empty > 0 {
						md.WriteString(fmt.Sprintf("    \"%s\" : %d\n", az, poolAZs[az].empty))
					}
				}
				md.WriteString("```\n\n")
			} else {
				md.WriteString("<!-- placeholder: No Empty Disruptions -->\n\n")
			}

			md.WriteString("<!-- /charts-row -->\n\n")

			// Instance type breakdown per AZ
			type instanceAZKey struct {
				instanceType string
				az           string
			}
			instanceAZCount := make(map[instanceAZKey]int)
			instanceAZInterrupted := make(map[instanceAZKey]int)
			instanceTypes := make(map[string]bool)

			for _, rec := range records {
				if rec.nodepool != pool || rec.zone == "" || rec.instanceType == "" {
					continue
				}
				key := instanceAZKey{instanceType: rec.instanceType, az: rec.zone}
				instanceAZCount[key]++
				if rec.interruptKind == "spot_interrupted" {
					instanceAZInterrupted[key]++
				}
				instanceTypes[rec.instanceType] = true
			}

			if len(instanceTypes) > 0 {
				sortedInstanceTypes := make([]string, 0, len(instanceTypes))
				for it := range instanceTypes {
					sortedInstanceTypes = append(sortedInstanceTypes, it)
				}
				sort.Strings(sortedInstanceTypes)

				// Count distinct instance types (capacity pools) per AZ
				capacityPoolsPerAZ := make(map[string]int)
				for _, az := range sortedPoolAZs {
					for _, it := range sortedInstanceTypes {
						key := instanceAZKey{instanceType: it, az: az}
						if instanceAZCount[key] > 0 {
							capacityPoolsPerAZ[az]++
						}
					}
				}
				totalCapacityPools := len(instanceTypes)

				md.WriteString("<!-- tables-row -->\n\n")

				// Total capacity pools
				md.WriteString(fmt.Sprintf("#### %s — Total Capacity Pools\n\n", pool))
				md.WriteString("| Metric | Value |\n")
				md.WriteString("|--------|-------|\n")
				md.WriteString(fmt.Sprintf("| Distinct Instance Types (all AZs) | %d |\n", totalCapacityPools))
				md.WriteString("\n")

				// Capacity Pools by AZ table
				md.WriteString(fmt.Sprintf("#### %s — Capacity Pools by AZ\n\n", pool))
				md.WriteString("| AZ | Distinct Instance Types |\n")
				md.WriteString("|----|------------------------|\n")
				for _, az := range sortedPoolAZs {
					md.WriteString(fmt.Sprintf("| %s | %d |\n", az, capacityPoolsPerAZ[az]))
				}
				md.WriteString("\n")

				// Instance Types by AZ table — sorted by total count descending
				type instanceTotal struct {
					name  string
					total int
				}
				var instanceTotals []instanceTotal
				for _, it := range sortedInstanceTypes {
					total := 0
					for _, az := range sortedPoolAZs {
						key := instanceAZKey{instanceType: it, az: az}
						total += instanceAZCount[key]
					}
					instanceTotals = append(instanceTotals, instanceTotal{name: it, total: total})
				}
				sort.Slice(instanceTotals, func(i, j int) bool {
					return instanceTotals[i].total > instanceTotals[j].total
				})

				maxVisible := 10
				writeInstanceTableHeader := func() {
					md.WriteString("| Instance Type |")
					for _, az := range sortedPoolAZs {
						md.WriteString(fmt.Sprintf(" %s |", az))
					}
					md.WriteString("\n|---|")
					for range sortedPoolAZs {
						md.WriteString("---|")
					}
					md.WriteString("\n")
				}
				writeInstanceRow := func(it string) {
					md.WriteString(fmt.Sprintf("| %s |", it))
					for _, az := range sortedPoolAZs {
						key := instanceAZKey{instanceType: it, az: az}
						count := instanceAZCount[key]
						interrupted := instanceAZInterrupted[key]
						if count == 0 {
							md.WriteString(" - |")
						} else if interrupted > 0 {
							md.WriteString(fmt.Sprintf(" %d (%d int.) |", count, interrupted))
						} else {
							md.WriteString(fmt.Sprintf(" %d |", count))
						}
					}
					md.WriteString("\n")
				}

				md.WriteString(fmt.Sprintf("#### %s — Instance Types by AZ\n\n", pool))
				writeInstanceTableHeader()
				for i, it := range instanceTotals {
					if i >= maxVisible {
						break
					}
					writeInstanceRow(it.name)
				}
				md.WriteString("\n")

				if len(instanceTotals) > maxVisible {
					md.WriteString(fmt.Sprintf("<!-- collapsible: Show %d more instance types -->\n\n", len(instanceTotals)-maxVisible))
					writeInstanceTableHeader()
					for i := maxVisible; i < len(instanceTotals); i++ {
						writeInstanceRow(instanceTotals[i].name)
					}
					md.WriteString("\n")
					md.WriteString("<!-- /collapsible -->\n\n")
				}

				md.WriteString("<!-- /tables-row -->\n\n")
			}

			// Timeline charts for this pool
			{
				type timeEvent struct {
					t  time.Time
					az string
				}
				var interruptEvents []timeEvent
				var underutilEvents []timeEvent
				var emptyEvents []timeEvent

				for _, rec := range records {
					if rec.nodepool != pool || rec.zone == "" {
						continue
					}
					if rec.interruptKind == "spot_interrupted" && rec.interruptionTime != "" {
						t, err := time.Parse(time.RFC3339Nano, rec.interruptionTime)
						if err == nil {
							interruptEvents = append(interruptEvents, timeEvent{t: t, az: rec.zone})
						}
					}
					if rec.disruptionReason == "underutilized" && rec.disruptionTime != "" {
						t, err := time.Parse(time.RFC3339Nano, rec.disruptionTime)
						if err == nil {
							underutilEvents = append(underutilEvents, timeEvent{t: t, az: rec.zone})
						}
					}
					if rec.disruptionReason == "empty" && rec.disruptionTime != "" {
						t, err := time.Parse(time.RFC3339Nano, rec.disruptionTime)
						if err == nil {
							emptyEvents = append(emptyEvents, timeEvent{t: t, az: rec.zone})
						}
					}
				}

				// Determine time range and bucket into 10-minute intervals
				var allTimes []time.Time
				for _, e := range interruptEvents {
					allTimes = append(allTimes, e.t)
				}
				for _, e := range underutilEvents {
					allTimes = append(allTimes, e.t)
				}
				for _, e := range emptyEvents {
					allTimes = append(allTimes, e.t)
				}

				if len(allTimes) > 0 {
					sort.Slice(allTimes, func(i, j int) bool { return allTimes[i].Before(allTimes[j]) })
					tMin := allTimes[0].Truncate(10 * time.Minute)
					tMax := allTimes[len(allTimes)-1].Truncate(10 * time.Minute).Add(10 * time.Minute)

					// Build time buckets
					var bucketLabels []string
					for t := tMin; t.Before(tMax); t = t.Add(10 * time.Minute) {
						bucketLabels = append(bucketLabels, t.Format("15:04"))
					}
					numBuckets := len(bucketLabels)

					bucketIdx := func(t time.Time) int {
						idx := int(t.Sub(tMin) / (10 * time.Minute))
						if idx >= numBuckets {
							idx = numBuckets - 1
						}
						if idx < 0 {
							idx = 0
						}
						return idx
					}

					// Only show full-hour labels on x-axis
					var xLabels []string
					for _, l := range bucketLabels {
						if strings.HasSuffix(l, ":00") {
							xLabels = append(xLabels, l)
						} else {
							xLabels = append(xLabels, "")
						}
					}

					// Spot interruption timeline
					if len(interruptEvents) > 0 {
						azBuckets := make(map[string][]int)
						for _, az := range sortedPoolAZs {
							azBuckets[az] = make([]int, numBuckets)
						}
						for _, e := range interruptEvents {
							azBuckets[e.az][bucketIdx(e.t)]++
						}

						md.WriteString(fmt.Sprintf("#### %s — Spot Interruptions Timeline\n\n", pool))
						md.WriteString("<!-- chartjs\n")
						md.WriteString("title: Spot Interruptions per 10min\n")
						md.WriteString(fmt.Sprintf("labels: %s\n", strings.Join(xLabels, ",")))
						for _, az := range sortedPoolAZs {
							md.WriteString(fmt.Sprintf("series: %s: %s\n", az, intSliceToString(azBuckets[az])))
						}
						md.WriteString("/chartjs -->\n\n")
					} else {
						md.WriteString(fmt.Sprintf("#### %s — Spot Interruptions Timeline\n\n", pool))
						md.WriteString("<!-- placeholder: No Spot Interruptions data -->\n\n")
					}

					// Underutilized disruption timeline
					if len(underutilEvents) > 0 {
						azBuckets := make(map[string][]int)
						for _, az := range sortedPoolAZs {
							azBuckets[az] = make([]int, numBuckets)
						}
						for _, e := range underutilEvents {
							azBuckets[e.az][bucketIdx(e.t)]++
						}

						md.WriteString(fmt.Sprintf("#### %s — Underutilized Disruptions Timeline\n\n", pool))
						md.WriteString("<!-- chartjs\n")
						md.WriteString("title: Underutilized Disruptions per 10min\n")
						md.WriteString(fmt.Sprintf("labels: %s\n", strings.Join(xLabels, ",")))
						for _, az := range sortedPoolAZs {
							md.WriteString(fmt.Sprintf("series: %s: %s\n", az, intSliceToString(azBuckets[az])))
						}
						md.WriteString("/chartjs -->\n\n")
					} else {
						md.WriteString(fmt.Sprintf("#### %s — Underutilized Disruptions Timeline\n\n", pool))
						md.WriteString("<!-- placeholder: No Underutilized Disruptions data -->\n\n")
					}

					// Empty disruption timeline
					if len(emptyEvents) > 0 {
						azBuckets := make(map[string][]int)
						for _, az := range sortedPoolAZs {
							azBuckets[az] = make([]int, numBuckets)
						}
						for _, e := range emptyEvents {
							azBuckets[e.az][bucketIdx(e.t)]++
						}

						md.WriteString(fmt.Sprintf("#### %s — Empty Disruptions Timeline\n\n", pool))
						md.WriteString("<!-- chartjs\n")
						md.WriteString("title: Empty Disruptions per 10min\n")
						md.WriteString(fmt.Sprintf("labels: %s\n", strings.Join(xLabels, ",")))
						for _, az := range sortedPoolAZs {
							md.WriteString(fmt.Sprintf("series: %s: %s\n", az, intSliceToString(azBuckets[az])))
						}
						md.WriteString("/chartjs -->\n\n")
					} else {
						md.WriteString(fmt.Sprintf("#### %s — Empty Disruptions Timeline\n\n", pool))
						md.WriteString("<!-- placeholder: No Empty Disruptions data -->\n\n")
					}
				} else {
					// No events at all for this pool
					md.WriteString(fmt.Sprintf("#### %s — Spot Interruptions Timeline\n\n", pool))
					md.WriteString("<!-- placeholder: No Spot Interruptions data -->\n\n")
					md.WriteString(fmt.Sprintf("#### %s — Underutilized Disruptions Timeline\n\n", pool))
					md.WriteString("<!-- placeholder: No Underutilized Disruptions data -->\n\n")
					md.WriteString(fmt.Sprintf("#### %s — Empty Disruptions Timeline\n\n", pool))
					md.WriteString("<!-- placeholder: No Empty Disruptions data -->\n\n")
				}
			}
		}
	}

	output := md.String()

	if outputFile == "" {
		fmt.Print(output)
	} else if strings.HasSuffix(outputFile, ".html") {
		html := renderHTML(output)
		if err := os.WriteFile(outputFile, []byte(html), 0644); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing %s: %v\n", outputFile, err)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "Written HTML report to %s\n", outputFile)
	} else {
		if err := os.WriteFile(outputFile, []byte(output), 0644); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing %s: %v\n", outputFile, err)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "Written statistics report to %s\n", outputFile)
	}
}

func renderHTML(markdown string) string {
	var html strings.Builder
	html.WriteString(`<!DOCTYPE html>
<html>
<head>
<meta charset="utf-8">
<title>Karpenter Statistics Report</title>
<style>
body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; max-width: 1200px; margin: 0 auto; padding: 2rem; background: #f8f9fa; }
h1 { color: #1a1a2e; }
h2 { color: #16213e; border-bottom: 2px solid #0f3460; padding-bottom: 0.3em; }
h3 { color: #0f3460; }
h4 { color: #533483; }
table { border-collapse: collapse; margin: 1em 0; }
th, td { border: 1px solid #ddd; padding: 8px 12px; text-align: left; }
th { background: #0f3460; color: white; }
tr:nth-child(even) { background: #f2f2f2; }
.mermaid { background: white; padding: 1rem; border-radius: 8px; margin: 1em 0; }
.charts-row { display: flex; gap: 1rem; flex-wrap: wrap; }
.charts-row .mermaid { flex: 1; min-width: 250px; max-width: 400px; }
.charts-row .chart-placeholder { flex: 1; min-width: 250px; max-width: 400px; display: flex; align-items: center; justify-content: center; min-height: 200px; }
.tables-row { display: flex; gap: 2rem; flex-wrap: wrap; align-items: flex-start; }
.tables-row .table-cell { flex: 1; min-width: 300px; }
.timeline-chart { background: white; padding: 1rem; border-radius: 8px; margin: 1em 0; max-width: 1100px; }
</style>
<script src="https://cdn.jsdelivr.net/npm/chart.js@4"></script>
</head>
<body>
`)

	lines := strings.Split(markdown, "\n")
	inMermaid := false
	inTable := false
	inChartJS := false
	inTablesRow := false
	inTableCell := false
	var chartTitle string
	var chartLabels []string
	type chartSeries struct {
		name string
		data []string
	}
	var chartSeriesList []chartSeries
	chartCount := 0

	colors := []string{"#e63946", "#4dabf7", "#2a9d8f"}

	for _, line := range lines {
		if strings.TrimSpace(line) == "<!-- chartjs" {
			inChartJS = true
			chartTitle = ""
			chartLabels = nil
			chartSeriesList = nil
			continue
		}
		if inChartJS && strings.TrimSpace(line) == "/chartjs -->" {
			inChartJS = false
			chartCount++
			canvasID := fmt.Sprintf("chart_%d", chartCount)
			html.WriteString(fmt.Sprintf("<div class=\"timeline-chart\"><canvas id=\"%s\"></canvas></div>\n", canvasID))
			html.WriteString("<script>\n")
			html.WriteString(fmt.Sprintf("new Chart(document.getElementById('%s'), {\n", canvasID))
			html.WriteString("  type: 'line',\n")
			html.WriteString("  data: {\n")
			// Labels as JSON array
			html.WriteString(fmt.Sprintf("    labels: [%s],\n", strings.Join(quoteLabels(chartLabels), ",")))
			html.WriteString("    datasets: [\n")
			for i, s := range chartSeriesList {
				color := colors[i%len(colors)]
				html.WriteString(fmt.Sprintf("      {label:'%s', data:[%s], borderColor:'%s', backgroundColor:'%s', tension:0.3, pointRadius:1},\n", s.name, strings.Join(s.data, ","), color, color))
			}
			html.WriteString("    ]\n")
			html.WriteString("  },\n")
			html.WriteString("  options: {\n")
			html.WriteString(fmt.Sprintf("    plugins: {title: {display:true, text:'%s'}},\n", chartTitle))
			html.WriteString("    scales: {\n")
			html.WriteString("      x: {title: {display:true, text:'Time (UTC)'}, ticks: {callback: function(val,idx) { var l=this.getLabelForValue(idx); return l||null; }, maxRotation:0}},\n")
			html.WriteString("      y: {title: {display:true, text:'Events per 10min'}, beginAtZero:true, ticks:{stepSize:1}}\n")
			html.WriteString("    }\n")
			html.WriteString("  }\n")
			html.WriteString("});\n")
			html.WriteString("</script>\n")
			continue
		}
		if inChartJS {
			if strings.HasPrefix(line, "title: ") {
				chartTitle = line[7:]
			} else if strings.HasPrefix(line, "labels: ") {
				chartLabels = strings.Split(line[8:], ",")
			} else if strings.HasPrefix(line, "series: ") {
				rest := line[8:]
				colonIdx := strings.Index(rest, ": ")
				if colonIdx > 0 {
					name := rest[:colonIdx]
					data := strings.Split(rest[colonIdx+2:], ", ")
					chartSeriesList = append(chartSeriesList, chartSeries{name: name, data: data})
				}
			}
			continue
		}

		if strings.TrimSpace(line) == "<!-- charts-row -->" {
			html.WriteString("<div class=\"charts-row\">\n")
			continue
		}
		if strings.TrimSpace(line) == "<!-- /charts-row -->" {
			html.WriteString("</div>\n")
			continue
		}
		if strings.TrimSpace(line) == "<!-- tables-row -->" {
			inTablesRow = true
			html.WriteString("<div class=\"tables-row\">\n")
			continue
		}
		if strings.TrimSpace(line) == "<!-- /tables-row -->" {
			if inTable {
				inTable = false
				html.WriteString("</table>\n")
			}
			if inTableCell {
				inTableCell = false
				html.WriteString("</div>\n")
			}
			inTablesRow = false
			html.WriteString("</div>\n")
			continue
		}

		if strings.HasPrefix(strings.TrimSpace(line), "<!-- collapsible:") {
			label := strings.TrimPrefix(strings.TrimSpace(line), "<!-- collapsible: ")
			label = strings.TrimSuffix(label, " -->")
			html.WriteString(fmt.Sprintf("<details><summary style=\"cursor:pointer;color:#0f3460;font-weight:bold;margin:0.5em 0\">%s</summary>\n", label))
			continue
		}
		if strings.TrimSpace(line) == "<!-- /collapsible -->" {
			if inTable {
				inTable = false
				html.WriteString("</table>\n")
			}
			html.WriteString("</details>\n")
			continue
		}

		if strings.HasPrefix(strings.TrimSpace(line), "<!-- placeholder:") {
			label := strings.TrimPrefix(strings.TrimSpace(line), "<!-- placeholder: ")
			label = strings.TrimSuffix(label, " -->")
			html.WriteString(fmt.Sprintf("<div class=\"chart-placeholder\"><em style=\"color:#999\">%s</em></div>\n", label))
			continue
		}

		if strings.HasPrefix(line, "```mermaid") {
			inMermaid = true
			html.WriteString("<div class=\"mermaid\">\n")
			continue
		}
		if inMermaid && strings.HasPrefix(line, "```") {
			inMermaid = false
			html.WriteString("</div>\n")
			continue
		}
		if inMermaid {
			html.WriteString(line + "\n")
			continue
		}

		if strings.HasPrefix(line, "| ") && !inTable {
			inTable = true
			html.WriteString("<table>\n")
			cells := splitTableRow(line)
			html.WriteString("<tr>")
			for _, c := range cells {
				html.WriteString("<th>" + c + "</th>")
			}
			html.WriteString("</tr>\n")
			continue
		}
		if inTable && strings.HasPrefix(line, "|--") {
			continue
		}
		if inTable && strings.HasPrefix(line, "| ") {
			cells := splitTableRow(line)
			html.WriteString("<tr>")
			for _, c := range cells {
				html.WriteString("<td>" + c + "</td>")
			}
			html.WriteString("</tr>\n")
			continue
		}
		if inTable && !strings.HasPrefix(line, "|") {
			inTable = false
			html.WriteString("</table>\n")
		}

		if strings.HasPrefix(line, "# ") {
			html.WriteString("<h1>" + line[2:] + "</h1>\n")
		} else if strings.HasPrefix(line, "## ") {
			html.WriteString("<h2>" + line[3:] + "</h2>\n")
		} else if strings.HasPrefix(line, "### ") {
			html.WriteString("<h3>" + line[4:] + "</h3>\n")
		} else if strings.HasPrefix(line, "#### ") {
			if inTablesRow {
				if inTable {
					inTable = false
					html.WriteString("</table>\n")
				}
				if inTableCell {
					html.WriteString("</div>\n")
				}
				inTableCell = true
				html.WriteString("<div class=\"table-cell\">\n")
			}
			html.WriteString("<h4>" + line[5:] + "</h4>\n")
		} else if strings.HasPrefix(line, "*") && strings.HasSuffix(line, "*") {
			html.WriteString("<p><em>" + strings.Trim(line, "*") + "</em></p>\n")
		}
	}

	if inTable {
		html.WriteString("</table>\n")
	}

	html.WriteString(`
<script type="module">
import mermaid from 'https://cdn.jsdelivr.net/npm/mermaid@11/dist/mermaid.esm.min.mjs';
mermaid.initialize({ startOnLoad: true, theme: 'default' });
</script>
</body>
</html>
`)
	return html.String()
}

func splitTableRow(line string) []string {
	line = strings.Trim(line, "|")
	parts := strings.Split(line, "|")
	var cells []string
	for _, p := range parts {
		cells = append(cells, strings.TrimSpace(p))
	}
	return cells
}

func sortedKeys(m map[string]*poolAZStats) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func quoteLabels(labels []string) []string {
	quoted := make([]string, len(labels))
	for i, l := range labels {
		quoted[i] = "\"" + l + "\""
	}
	return quoted
}

func intSliceToString(vals []int) string {
	parts := make([]string, len(vals))
	for i, v := range vals {
		parts[i] = fmt.Sprintf("%d", v)
	}
	return strings.Join(parts, ", ")
}
