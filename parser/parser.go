// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0
package parser

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/nav-inc/datetime"
)

// var header string = "nodeclaim,createdtime,nodepool,instancetypes,launchedtime,providerid,instancetype,zone,capacitytype,registeredtime,k8snodename,initializedtime,nodereadytime,nodereadytimesec,disruptiontime,disruptionreason,disruptiondecision,disruptednodecount,replacementnodecount,disruptedpodcount,annotationtime,annotation,tainttime,taint,interruptiontime,interruptionkind,deletedtime,nodeterminationtime,nodeterminationtimesec,nodelifecycletime,nodelifecycletimesec,initialized,deleted"
var header string

var (
	replacer                 = strings.NewReplacer(", ", "|", " ", "", "(s)", "s")
	messagePattern           = regexp.MustCompile(`"message":"(.*)","commit"`)
	createdPattern           = regexp.MustCompile(`"time":"(.*)","logger".*"NodePool":{"name":"(.*)"},"NodeClaim":{"name":"(.*)"},"requests".*"instance-types":"(.*)"`)
	launchedPattern          = regexp.MustCompile(`"time":"(.*)","logger".*"NodeClaim":{"name":"(.*)"},.*"provider-id":"(.*)","instance-type":"(.*)","zone":"(.*)","capacity-type":"(.*)","allocatable"`)
	registeredPattern        = regexp.MustCompile(`"time":"(.*)","logger".*"NodeClaim":{"name":"(.*)"},.*,"Node":{"name":"(.*)"`)
	initializedPattern       = regexp.MustCompile(`"time":"(.*)","logger".*"NodeClaim":{"name":"(.*)"},"namespace"`)
	disruptingReasonPattern  = regexp.MustCompile(`"time":"(.*)","logger".*"reason":"(.*)","decision":"(.*)","disrupted-node-count":(.*),"replacement-node-count":(.*),"pod-count":(.*),"disrupted-nodes":.*,"NodeClaim":{"name":"(.*)"},"capacity-type"`)
	disruptingCommandPattern = regexp.MustCompile(`"time":"(.*)","logger".*"command":"(.*)","decision":"(.*)","disrupted-node-count":(.*),"replacement-node-count":(.*),"pod-count":(.*),"disrupted-nodes":.*,"NodeClaim":{"name":"(.*)"},"capacity-type"`)
	interruptionPattern      = regexp.MustCompile(`"time":"(.*)","logger".*"messageKind":"(.*)","NodeClaim":{"name":"(.*)"},"action"`)
	annotatedPattern         = regexp.MustCompile(`"time":"(.*)","logger".*"NodeClaim":{"name":"(.*)"},"namespace".*,"(.*)":"(.*)"`)
	taintedNCPattern         = regexp.MustCompile(`"time":"(.*)","logger".*"NodeClaim":{"name":"(.*)"},"taint.Key":"(.*)","taint.Value":"(.*)","taint.Effect":"(.*)"`)
	taintedNodePattern       = regexp.MustCompile(`"time":"(.*)","logger".*"Node":{"name":"(.*)"},"namespace".*,"taint.Key":"(.*)","taint.Value":"(.*)","taint.Effect":"(.*)"`)
	taintedNodeSimplePattern = regexp.MustCompile(`"time":"(.*)","logger".*"Node":{"name":"(.*)"},"namespace"`)
	deletedPattern           = regexp.MustCompile(`"time":"(.*)","logger".*"NodeClaim":{"name":"(.*)"},"namespace"`)
)

// export all struct values because this is required for usage with packages like JSON encoding/decoding or reflect
// keep disruptednodecount, replacementnodecount, disruptedpodcount as strings because then we can have empty string ("") to differ from real values
type Nodeclaimstruct struct {
	Createdtime            string
	Nodepool               string
	Instancetypes          string
	Launchedtime           string
	Providerid             string
	Instancetype           string
	Zone                   string
	Capacitytype           string
	Registeredtime         string
	K8snodename            string
	Initializedtime        string
	Nodereadytime          time.Duration
	Nodereadytimesec       float64
	Disruptiontime         string
	Disruptionreason       string
	Disruptiondecision     string
	Disruptednodecount     string
	Replacementnodecount   string
	Disruptedpodcount      string
	Annotationtime         string
	Annotation             string
	Tainttime              string
	Taint                  string
	Interruptiontime       string
	Interruptionkind       string
	Deletedtime            string
	Nodeterminationtime    time.Duration
	Nodeterminationtimesec float64
	Nodelifecycletime      time.Duration
	Nodelifecycletimesec   float64
	Initialized            bool
	Deleted                bool
}

// internal helper function for pattern matching
func matchPattern(pattern *regexp.Regexp, logline string) []string {
	return pattern.FindStringSubmatch(logline)
}

// internal helper function for scanner error handling
func scannerErr(scanner *bufio.Scanner, stdin string) {
	// Ctrl-C will always lead to "http2: response body closed", so suppress this error
	if err := scanner.Err(); err != nil {
		if err.Error() != "http2: response body closed" {
			fmt.Fprintf(os.Stderr, "Error \"%s\" parsing %s\n", err, stdin)
		}
	}
}

// wrapper around main parsing logic with blocking
func BlockingParser(ch chan os.Signal, scanner *bufio.Scanner, nodeclaimmap *map[string]Nodeclaimstruct, k8snodenamemap *map[string]string, stdin string, inputline int) {
	// main parsing logic
	for scanner.Scan() {
		//logline := scanner.Text()
		ParseKarpenterLogs(scanner.Text(), nodeclaimmap, k8snodenamemap, stdin, inputline)
		// we wait until Ctrl-C because we have an input from something like "kubectl logs -n karpenter -l=app.kubernetes.io/name=karpenter -f"
		go func() {
			<-ch
		}()
	}
	scannerErr(scanner, stdin)
}

// wrapper around main parsing logic without blocking
func NonBlockingParser(scanner *bufio.Scanner, nodeclaimmap *map[string]Nodeclaimstruct, k8snodenamemap *map[string]string, stdin string, inputline int) {
	// main parsing logic
	for scanner.Scan() {
		//logline := scanner.Text()
		ParseKarpenterLogs(scanner.Text(), nodeclaimmap, k8snodenamemap, stdin, inputline)
	}
	scannerErr(scanner, stdin)
}

// main parsing logic
func ParseKarpenterLogs(logline string, nodeclaimmap *map[string]Nodeclaimstruct, k8snodenamemap *map[string]string, filename string, inputline int) {
	var createdtime, nodepool, instancetypes, nodeclaim string
	var matchslice []string

	inputline++
	matchslice = messagePattern.FindStringSubmatch(logline)
	// process matchslice if we found a match
	if matchslice != nil {
		//fmt.Println("message: ", matchslice[1])
		switch matchslice[1] {
		case "created nodeclaim":
			// extract time and nodeclaim (new one)
			if matchslicesub := matchPattern(createdPattern, logline); matchslicesub != nil {
				//matchslicesub[0] always contains whole logline
				for i, val := range matchslicesub[1:] {
					switch i {
					case 0:
						createdtime = val
					case 1:
						nodepool = val
					case 2:
						nodeclaim = val
					case 3:
						// substitute "," because we output CSV finally
						// Karpenter provisioner.go prints the first 5 instance types only and remaining number
						if idx := strings.LastIndex(val, " and "); idx > 0 {
							instancetypes = fmt.Sprintf("%s|%s", replacer.Replace(val[:idx]), replacer.Replace(val[idx:]))
						} else {
							instancetypes = replacer.Replace(val)
						}
					}
				}
				// we only create a new nodeclaimmap map entry when we capture a "created nodeclaim" log line
				// add entry to hash map
				(*nodeclaimmap)[nodeclaim] = Nodeclaimstruct{
					Createdtime:            createdtime,
					Nodepool:               nodepool,
					Instancetypes:          instancetypes,
					Launchedtime:           "",
					Providerid:             "",
					Instancetype:           "",
					Zone:                   "",
					Capacitytype:           "",
					Registeredtime:         "",
					K8snodename:            "",
					Initializedtime:        "",
					Nodereadytime:          0,
					Nodereadytimesec:       0.0,
					Disruptiontime:         "",
					Disruptionreason:       "",
					Disruptiondecision:     "",
					Disruptednodecount:     "",
					Replacementnodecount:   "",
					Disruptedpodcount:      "",
					Annotationtime:         "",
					Annotation:             "",
					Tainttime:              "",
					Taint:                  "",
					Interruptionkind:       "",
					Deletedtime:            "",
					Nodeterminationtime:    0,
					Nodeterminationtimesec: 0.0,
					Nodelifecycletime:      0,
					Nodelifecycletimesec:   0.0,
					Initialized:            false,
					Deleted:                false,
				}
			} else {
				fmt.Fprintf(os.Stderr, "Parsing error for message \"%s\" in line %d in %s, probably Karpenter log syntax has changed!\n", matchslice[1], inputline, filename)
			}
		case "launched nodeclaim":
			// extract all nodeclaim details here
			if matchslicesub := matchPattern(launchedPattern, logline); matchslicesub != nil {
				//matchslicesub[0] always contains whole logline
				// if logline parsing went well, matchslicesub[2] will contain NodeClaim
				if nodeclaim = matchslicesub[2]; nodeclaim == "" {
					fmt.Fprintf(os.Stderr, "Parsing error empty \"NodeClaim\" for message \"%s\" in line %d in %s, probably Karpenter log syntax has changed!\n", matchslice[1], inputline, filename)
				} else if entry, ok := (*nodeclaimmap)[nodeclaim]; ok {
					//matchslice[0] always contains whole logline
					entry.Launchedtime = matchslicesub[1]
					awsproviderID := strings.Split(matchslicesub[3], "/")
					entry.Providerid = awsproviderID[len(awsproviderID)-1]
					entry.Instancetype = matchslicesub[4]
					entry.Zone = matchslicesub[5]
					entry.Capacitytype = matchslicesub[6]
					(*nodeclaimmap)[nodeclaim] = entry
				}
			} else {
				fmt.Fprintf(os.Stderr, "Parsing error for message \"%s\" in line %d in %s, probably Karpenter log syntax has changed!\n", matchslice[1], inputline, filename)
			}
		case "registered nodeclaim":
			// extract time, nodeclaim and K8s node name
			if matchslicesub := matchPattern(registeredPattern, logline); matchslicesub != nil {
				//matchslicesub[0] always contains whole logline
				// if logline parsing went well, matchslicesub[2] will contain NodeClaim
				if nodeclaim = matchslicesub[2]; nodeclaim == "" {
					fmt.Fprintf(os.Stderr, "Parsing error empty \"NodeClaim\" for message \"%s\" in line %d in %s, probably Karpenter log syntax has changed!\n", matchslice[1], inputline, filename)
				} else if entry, ok := (*nodeclaimmap)[nodeclaim]; ok {
					//matchslicesub[0] always contains whole logline
					entry.Registeredtime = matchslicesub[1]
					entry.K8snodename = matchslicesub[3]
					(*k8snodenamemap)[matchslicesub[3]] = nodeclaim
					(*nodeclaimmap)[nodeclaim] = entry
				}
			} else {
				fmt.Fprintf(os.Stderr, "Parsing error for message \"%s\" in line %d in %s, probably Karpenter log syntax has changed!\n", matchslice[1], inputline, filename)
			}
		case "initialized nodeclaim":
			// extract time and nodeclaim
			if matchslicesub := matchPattern(initializedPattern, logline); matchslicesub != nil {
				//matchslicesub[0] always contains whole logline
				// if logline parsing went well, matchslicesub[2] will contain NodeClaim
				if nodeclaim = matchslicesub[2]; nodeclaim == "" {
					fmt.Fprintf(os.Stderr, "Parsing error empty \"NodeClaim\" for message \"%s\" in line %d in %s, probably Karpenter log syntax has changed!\n", matchslice[1], inputline, filename)
				} else if entry, ok := (*nodeclaimmap)[nodeclaim]; ok {
					//matchslicesub[0] always contains whole logline
					if entry.Initializedtime = matchslicesub[1]; entry.Initializedtime != "" {
						// calculate node startup time
						if entry.Createdtime != "" {
							t1, _ := datetime.Parse(entry.Createdtime, time.UTC)
							t2, _ := datetime.Parse(entry.Initializedtime, time.UTC)
							entry.Nodereadytime = t2.Sub(t1)
							entry.Nodereadytimesec = entry.Nodereadytime.Seconds()
						}
					} else {
						fmt.Fprintf(os.Stderr, "Parsing error empty \"initialized time\" for message \"%s\" in line %d in %s, probably Karpenter log syntax has changed!\n", matchslice[1], inputline, filename)
					}
					// we set nodeclaim to deleted even if we (for whatever reason) could not extract time
					entry.Initialized = true
					(*nodeclaimmap)[nodeclaim] = entry
				}
			} else {
				fmt.Fprintf(os.Stderr, "Parsing error for message \"%s\" in line %d in %s, probably Karpenter log syntax has changed!\n", matchslice[1], inputline, filename)
			}
		case "disrupting node(s)":
			// extract time, message reason/command, decision, disrupted-node-count, replacment-node-count, podcount and nodeclaim
			var matchslicesub []string
			var isCommandField bool
			if matchslicesub = matchPattern(disruptingReasonPattern, logline); matchslicesub != nil {
				isCommandField = false
			} else if matchslicesub = matchPattern(disruptingCommandPattern, logline); matchslicesub != nil {
				isCommandField = true
			}
			if matchslicesub != nil {
				if nodeclaim = matchslicesub[7]; nodeclaim == "" {
					fmt.Fprintf(os.Stderr, "Parsing error empty \"NodeClaim\" for message \"%s\" in line %d in %s, probably Karpenter log syntax has changed!\n", matchslice[1], inputline, filename)
				} else if entry, ok := (*nodeclaimmap)[nodeclaim]; ok {
					entry.Disruptiontime = matchslicesub[1]
					if isCommandField {
						if idx := strings.IndexByte(matchslicesub[2], '/'); idx > 0 {
							entry.Disruptionreason = strings.ToLower(matchslicesub[2][:idx])
						}
					} else {
						entry.Disruptionreason = matchslicesub[2]
					}
					entry.Disruptiondecision = matchslicesub[3]
					entry.Disruptednodecount = matchslicesub[4]
					entry.Replacementnodecount = matchslicesub[5]
					entry.Disruptedpodcount = matchslicesub[6]
					(*nodeclaimmap)[nodeclaim] = entry
				}
			} else {
				fmt.Fprintf(os.Stderr, "Parsing error for message \"%s\" in line %d in %s, probably Karpenter log syntax has changed!\n", matchslice[1], inputline, filename)
			}
		case "initiating delete from interruption message":
			// extract time, message kind (interruption kind/reason) and nodeclaim (this message kind has NodeClaim in a different position!)
			if matchslicesub := matchPattern(interruptionPattern, logline); matchslicesub != nil {
				//matchslicesub[0] always contains whole logline
				// if logline parsing went well, matchslicesub[3] will contain NodeClaim
				if nodeclaim = matchslicesub[3]; nodeclaim == "" {
					fmt.Fprintf(os.Stderr, "Parsing error empty \"NodeClaim\" for message \"%s\" in line %d in %s, probably Karpenter log syntax has changed!\n", matchslice[1], inputline, filename)
				} else if entry, ok := (*nodeclaimmap)[nodeclaim]; ok {
					entry.Interruptiontime = matchslicesub[1]
					entry.Interruptionkind = matchslicesub[2]
					(*nodeclaimmap)[nodeclaim] = entry
				}
			} else {
				fmt.Fprintf(os.Stderr, "Parsing error for message \"%s\" in line %d in %s, probably Karpenter log syntax has changed!\n", matchslice[1], inputline, filename)
			}
		case "annotated nodeclaim":
			// extract time, nodeclaim and annotation key/value
			if matchslicesub := matchPattern(annotatedPattern, logline); matchslicesub != nil {
				//matchslicesub[0] always contains whole logline
				// if logline parsing went well, matchslicesub[2] will contain NodeClaim
				if nodeclaim = matchslicesub[2]; nodeclaim == "" {
					fmt.Fprintf(os.Stderr, "Parsing error empty \"NodeClaim\" for message \"%s\" in line %d in %s, probably Karpenter log syntax has changed!\n", matchslice[1], inputline, filename)
				} else if entry, ok := (*nodeclaimmap)[nodeclaim]; ok {
					entry.Annotationtime = matchslicesub[1]
					entry.Annotation = fmt.Sprintf("%s:%s", matchslicesub[3], matchslicesub[4])
					(*nodeclaimmap)[nodeclaim] = entry
				}
			} else {
				fmt.Fprintf(os.Stderr, "Parsing error for message \"%s\" in line %d in %s, probably Karpenter log syntax has changed!\n", matchslice[1], inputline, filename)
			}
		case "tainted node":
			// extract time, nodeclaim and taint key/value/effect for Karpenter version 1.1.x+
			if matchslicesub := matchPattern(taintedNCPattern, logline); matchslicesub != nil {
				//matchslicesub[0] always contains whole logline
				// if logline parsing went well, matchslicesub[2] will contain NodeClaim
				if nodeclaim = matchslicesub[2]; nodeclaim == "" {
					fmt.Fprintf(os.Stderr, "Parsing error empty \"NodeClaim\" for message \"%s\" in line %d in %s, probably Karpenter log syntax has changed!\n", matchslice[1], inputline, filename)
				} else if entry, ok := (*nodeclaimmap)[nodeclaim]; ok {
					entry.Tainttime = matchslicesub[1]
					entry.Taint = fmt.Sprintf("%s:%s:%s", matchslicesub[3], matchslicesub[4], matchslicesub[5])
					(*nodeclaimmap)[nodeclaim] = entry
				}
			} else {
				// Karpenter version 0.37.x and 1.0.x don't put nodeclaim into "tainted node" message !
				// extract time, k8snodename taint key/value/effect for Karpenter version 1.0.x
				if matchslicesub := matchPattern(taintedNodePattern, logline); matchslicesub != nil {
					// if logline parsing went well, matchslicesub[2] will contain K8s node name
					if k8snodename := matchslicesub[2]; k8snodename == "" {
						fmt.Fprintf(os.Stderr, "Parsing error empty \"K8s node name\" for message \"%s\" in line %d in %s, probably Karpenter log syntax has changed!\n", matchslice[1], inputline, filename)
					} else if nodeclaim, ok := (*k8snodenamemap)[k8snodename]; ok {
						if entry, ok := (*nodeclaimmap)[nodeclaim]; ok {
							entry.Tainttime = matchslicesub[1]
							entry.Taint = fmt.Sprintf("%s:%s:%s", matchslicesub[3], matchslicesub[4], matchslicesub[5])
							(*nodeclaimmap)[nodeclaim] = entry
						}
					} else {
						fmt.Fprintf(os.Stderr, "No corresponding \"NodeClaim\" for K8s node \"%s\" for message \"tainted node\" in line %d in %s\n", k8snodename, inputline, filename)
						fmt.Fprintf(os.Stderr, "Most probably %s does not contain a corresponding \"created nodeclaim\" log entry\n", filename)
					}
				} else if matchslicesub := matchPattern(taintedNodeSimplePattern, logline); matchslicesub != nil {
					// Karpenter version 0.37.x don't put taint key/value/effect into "tainted node" message !
					// extract time and k8snodename for Karpenter version 0.37
					if k8snodename := matchslicesub[2]; k8snodename != "" {
						if nodeclaim, ok := (*k8snodenamemap)[k8snodename]; ok {
							if entry, ok := (*nodeclaimmap)[nodeclaim]; ok {
								entry.Tainttime = matchslicesub[1]
								(*nodeclaimmap)[nodeclaim] = entry
							}
						}
					}
				} else {
					fmt.Fprintf(os.Stderr, "Parsing error for message \"%s\" in line %d in %s, probably Karpenter log syntax has changed!\n", matchslice[1], inputline, filename)
				}
			}
		case "deleted nodeclaim":
			// extract time and nodeclaim
			if matchslicesub := matchPattern(deletedPattern, logline); matchslicesub != nil {
				//matchslicesub[0] always contains whole logline
				// if logline parsing went well, matchslicesub[2] will contain NodeClaim
				if nodeclaim = matchslicesub[2]; nodeclaim == "" {
					fmt.Fprintf(os.Stderr, "Parsing error empty \"NodeClaim\" for message \"%s\" in line %d in %s, probably Karpenter log syntax has changed!\n", matchslice[1], inputline, filename)
				} else if entry, ok := (*nodeclaimmap)[nodeclaim]; ok {
					//matchslicesub[0] always contains whole logline
					if entry.Deletedtime = matchslicesub[1]; entry.Deletedtime != "" {
						// calculate node lifecycle time
						if entry.Createdtime != "" {
							t1, _ := datetime.Parse(entry.Createdtime, time.UTC)
							t2, _ := datetime.Parse(entry.Deletedtime, time.UTC)
							entry.Nodelifecycletime = t2.Sub(t1)
							entry.Nodelifecycletimesec = entry.Nodelifecycletime.Seconds()
						}
						// calculate node termination time (time it takes from lifecycle annotation to actual deletion)
						// if this takes really long you might have some blocking PDB or taints
						if entry.Annotationtime != "" {
							t1, _ := datetime.Parse(entry.Annotationtime, time.UTC)
							t2, _ := datetime.Parse(entry.Deletedtime, time.UTC)
							entry.Nodeterminationtime = t2.Sub(t1)
							entry.Nodeterminationtimesec = entry.Nodeterminationtime.Seconds()
						}
					} else {
						fmt.Fprintf(os.Stderr, "Parsing error empty \"deleted time\" for message \"%s\" in line %d in %s, probably Karpenter log syntax has changed!\n", matchslice[1], inputline, filename)
					}
					// we set nodeclaim to deleted even if we (for whatever reason) could not extract time
					entry.Deleted = true
					(*nodeclaimmap)[nodeclaim] = entry
				}
			} else {
				fmt.Fprintf(os.Stderr, "Parsing error for message \"%s\" in line %d in %s, probably Karpenter log syntax has changed!\n", matchslice[1], inputline, filename)
			}
		}
	}
}
