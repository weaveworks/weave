package main

import (
	"fmt"
	"os"
	"sort"

	"golang.org/x/tools/cover"
)

func merge(p1, p2 *cover.Profile) *cover.Profile {
	output := cover.Profile{
		FileName: p1.FileName,
		Mode:     p1.Mode,
	}

	i, j := 0, 0
	for i < len(p1.Blocks) && j < len(p2.Blocks) {
		bi, bj := p1.Blocks[i], p2.Blocks[j]
		if bi.StartLine == bj.StartLine && bi.StartCol == bj.StartCol {

			if bi.EndLine != bj.EndLine ||
				bi.EndCol != bj.EndCol ||
				bi.NumStmt != bj.NumStmt {
				panic("Not run on same source!")
			}

			output.Blocks = append(output.Blocks, cover.ProfileBlock{
				StartLine: bi.StartLine,
				StartCol:  bi.StartCol,
				EndLine:   bi.EndLine,
				EndCol:    bi.EndCol,
				NumStmt:   bi.NumStmt,
				Count:     bi.Count + bj.Count,
			})
			i++
			j++
		} else if bi.StartLine < bj.StartLine || bi.StartLine == bj.StartLine && bi.StartCol < bj.StartCol {
			output.Blocks = append(output.Blocks, bi)
			i++
		} else {
			output.Blocks = append(output.Blocks, bj)
			j++
		}
	}

	for ; i < len(p1.Blocks); i++ {
		output.Blocks = append(output.Blocks, p1.Blocks[i])
	}

	for ; j < len(p2.Blocks); j++ {
		output.Blocks = append(output.Blocks, p2.Blocks[j])
	}

	return &output
}

func print(profiles []*cover.Profile) {
	fmt.Println("mode: atomic")
	for _, profile := range profiles {
		for _, block := range profile.Blocks {
			fmt.Printf("%s:%d.%d,%d.%d %d %d\n", profile.FileName, block.StartLine, block.StartCol,
				block.EndLine, block.EndCol, block.NumStmt, block.Count)
		}
	}
}

// Copied from https://github.com/golang/tools/blob/master/cover/profile.go
type byFileName []*cover.Profile

func (p byFileName) Len() int           { return len(p) }
func (p byFileName) Less(i, j int) bool { return p[i].FileName < p[j].FileName }
func (p byFileName) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }

func main() {
	outputProfiles := map[string]*cover.Profile{}
	for _, input := range os.Args[1:] {
		inputProfiles, err := cover.ParseProfiles(input)
		if err != nil {
			panic(fmt.Sprintf("Error parsing %s: %v", input, err))
		}
		for _, ip := range inputProfiles {
			op := outputProfiles[ip.FileName]
			if op == nil {
				outputProfiles[ip.FileName] = ip
			} else {
				outputProfiles[ip.FileName] = merge(op, ip)
			}
		}
	}
	profiles := make([]*cover.Profile, 0, len(outputProfiles))
	for _, profile := range outputProfiles {
		profiles = append(profiles, profile)
	}
	sort.Sort(byFileName(profiles))
	print(profiles)
}
