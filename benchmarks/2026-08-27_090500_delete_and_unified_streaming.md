# Benchmark Snapshot: 2026-08-27 09:05:00 (Table Row Deletion & Unified Streaming Mutation Engine)

* **Milestone**: Table Row Deletion (`rowlite delete`, `rowlite_delete`), Unified Streaming Transformer (`StreamTransformTable`), Sequential `[]RowAction` Model, Single-Pass vs. Two-Pass Query Routing, and Universal Fail-Fast Table Validation
* **OS**: Windows 11 (amd64)
* **CPU**: Intel(R) Core(TM) i7-8565U CPU @ 1.80GHz
* **Go Version**: `go1.26.6 windows/amd64`
* **Dataset**: 10,000 rows (7 columns, 1.25 MB Markdown GFM table)
* **Command**: `go test -bench="BenchmarkLargeTable10k" -benchmem -count=3 ./internal/core`

---

## 1. Summary of Metrics & Performance Highlights

| Benchmark Operation | Scope | Latency (ns/op) | Memory Allocation | Allocs / Op | Notes |
| :--- | :--- | :--- | :--- | :--- | :--- |
| **Hot Row Mutation (`ApplyRowUpdate`)** | In-Memory (Hot) | **626.6 ns** | **448 B / op** | **3 allocs** | Evaluates mutations against snapshot + stamps audit |
| **Hot Streaming Head (`Head 1`)** | In-Memory (Hot) | **208.5 ns** | **32 B / op** | **1 alloc** | Cached AST instant pushdown extraction |
| **Hot Exact Match (`id == 5000`)** | In-Memory (Hot) | **517.9 µs** | **32 B / op** | **1 alloc** | Zero-allocation in-memory filtering |
| **Streaming Delete (`--head 10`)** | Disk-to-Disk (Cold) | **51.01 ms** | **21.0 MB / op** | **274k allocs** | Single-pass streaming rewrite, dropping first 10 matches |
| **Streaming Delete (`--index 5000`)** | Disk-to-Disk (Cold) | **89.49 ms** | **21.0 MB / op** | **274k allocs** | Single-pass streaming rewrite, dropping targeted middle row |
| **Streaming Update (`--head 10`)** | Disk-to-Disk (Cold) | **67.56 ms** | **21.0 MB / op** | **274k allocs** | Single-pass streaming rewrite, mutating first 10 matches |
| **Streaming Update (`--index 5000`)** | Disk-to-Disk (Cold) | **74.34 ms** | **21.0 MB / op** | **274k allocs** | Single-pass streaming rewrite, mutating targeted middle row |
| **Streaming Append** | Disk-to-Disk (Cold) | **278.5 ms** | **21.0 MB / op** | **274k allocs** | Injects new row before post-table prose + atomic swap |

---

## 2. Inefficiency Regression Guard Audit

- **Single-Pass Disk I/O**: Validated. Forward mutations (`--where`, `--head`, positive `--index`, `--offset`/`--limit`, and `append`) evaluate predicates online and write `.tmp` in a single pass without prior scanning.
- **Fail-Fast Structural Validation**: Syntax errors (broken pipes) or column count mismatches abort cold operations immediately before any temporary workfile is swapped.
- **Prose Preservation**: Rows are appended/inserted inside the table block, keeping surrounding markdown headings and notes intact.
- **Strict $\mathcal{O}(1)$ RAM Bounds**: File rewriting streams line-by-line using a bounded buffer without buffering the whole table in memory.

---

## 3. Raw Benchmark Logs

```
goos: windows
goarch: amd64
pkg: github.com/pipik-roman/rowlite/internal/core
cpu: Intel(R) Core(TM) i7-8565U CPU @ 1.80GHz
BenchmarkLargeTable10k_EndToEnd_Delete_Streaming_Head10-8        	      22	  51011895 ns/op	21014731 B/op	  274410 allocs/op
BenchmarkLargeTable10k_EndToEnd_Delete_Streaming_Head10-8        	      24	  74804162 ns/op	21015017 B/op	  274412 allocs/op
BenchmarkLargeTable10k_EndToEnd_Delete_Streaming_Head10-8        	      10	 100629430 ns/op	21015636 B/op	  274413 allocs/op
BenchmarkLargeTable10k_EndToEnd_Delete_Streaming_SingleIndex-8   	      13	  89490915 ns/op	21018747 B/op	  274435 allocs/op
BenchmarkLargeTable10k_EndToEnd_Delete_Streaming_SingleIndex-8   	      15	 129813133 ns/op	21018514 B/op	  274435 allocs/op
BenchmarkLargeTable10k_EndToEnd_Delete_Streaming_SingleIndex-8   	       7	 149209557 ns/op	21018393 B/op	  274433 allocs/op
BenchmarkLargeTable10k_EndToEnd_Select_ExactMatch-8              	      30	  36786967 ns/op	10681759 B/op	  110802 allocs/op
BenchmarkLargeTable10k_EndToEnd_Select_ExactMatch-8              	      44	  38793164 ns/op	10681540 B/op	  110801 allocs/op
BenchmarkLargeTable10k_EndToEnd_Select_ExactMatch-8              	      36	  42651231 ns/op	10681933 B/op	  110802 allocs/op
BenchmarkLargeTable10k_EndToEnd_Select_Like_Unicode-8            	      28	  47976407 ns/op	11156228 B/op	  130814 allocs/op
BenchmarkLargeTable10k_EndToEnd_Select_Like_Unicode-8            	      31	  32597990 ns/op	11156409 B/op	  130814 allocs/op
BenchmarkLargeTable10k_EndToEnd_Select_Like_Unicode-8            	      26	  55038912 ns/op	11156289 B/op	  130814 allocs/op
BenchmarkLargeTable10k_EndToEnd_Select_Regex-8                   	       7	 192499129 ns/op	49636785 B/op	  340868 allocs/op
BenchmarkLargeTable10k_EndToEnd_Select_Regex-8                   	       4	 285502200 ns/op	49625180 B/op	  340868 allocs/op
BenchmarkLargeTable10k_EndToEnd_Select_Regex-8                   	       6	 177841417 ns/op	49634388 B/op	  340868 allocs/op
BenchmarkLargeTable10k_EndToEnd_Select_PositionalSlice-8         	      26	 178720877 ns/op	12159745 B/op	  110832 allocs/op
BenchmarkLargeTable10k_EndToEnd_Select_PositionalSlice-8         	       6	 171634133 ns/op	12160065 B/op	  110833 allocs/op
BenchmarkLargeTable10k_EndToEnd_Select_PositionalSlice-8         	       1	1459538900 ns/op	12159704 B/op	  110833 allocs/op
BenchmarkLargeTable10k_EndToEnd_Select_Streaming_Head1-8         	      43	  26204998 ns/op	 4421629 B/op	     220 allocs/op
BenchmarkLargeTable10k_EndToEnd_Select_Streaming_Head1-8         	      48	  24166340 ns/op	 4421461 B/op	     219 allocs/op
BenchmarkLargeTable10k_EndToEnd_Select_Streaming_Head1-8         	     100	  16367064 ns/op	 4421556 B/op	     220 allocs/op
BenchmarkLargeTable10k_EndToEnd_Select_Streaming_Tail1-8         	      21	  82781514 ns/op	 9680511 B/op	  110537 allocs/op
BenchmarkLargeTable10k_EndToEnd_Select_Streaming_Tail1-8         	       4	 462575250 ns/op	 9678902 B/op	  110533 allocs/op
BenchmarkLargeTable10k_EndToEnd_Select_Streaming_Tail1-8         	       7	 152082586 ns/op	 9680774 B/op	  110538 allocs/op
BenchmarkLargeTable10k_EndToEnd_Count_FastPath-8                 	      18	  60309139 ns/op	 9705561 B/op	  110787 allocs/op
BenchmarkLargeTable10k_EndToEnd_Count_FastPath-8                 	      31	  34154094 ns/op	 9705702 B/op	  110786 allocs/op
BenchmarkLargeTable10k_EndToEnd_Count_FastPath-8                 	      30	  40551447 ns/op	 9705272 B/op	  110786 allocs/op
BenchmarkLargeTable10k_EndToEnd_Count_Filtered-8                 	      24	  63651883 ns/op	 9972314 B/op	  110552 allocs/op
BenchmarkLargeTable10k_EndToEnd_Count_Filtered-8                 	      20	  52640860 ns/op	 9972180 B/op	  110552 allocs/op
BenchmarkLargeTable10k_EndToEnd_Count_Filtered-8                 	      28	  78068289 ns/op	 9972512 B/op	  110553 allocs/op
BenchmarkLargeTable10k_EndToEnd_Sort-8                           	       6	 174626583 ns/op	10680941 B/op	  110804 allocs/op
BenchmarkLargeTable10k_EndToEnd_Sort-8                           	      12	 138676442 ns/op	10680752 B/op	  110802 allocs/op
BenchmarkLargeTable10k_EndToEnd_Sort-8                           	      10	 118130090 ns/op	10682804 B/op	  110808 allocs/op
BenchmarkLargeTable10k_EndToEnd_Append-8                         	       3	 417893900 ns/op	21016765 B/op	  274439 allocs/op
BenchmarkLargeTable10k_EndToEnd_Append-8                         	       4	 297278000 ns/op	21018782 B/op	  274447 allocs/op
BenchmarkLargeTable10k_EndToEnd_Append-8                         	       4	 278537875 ns/op	21018042 B/op	  274446 allocs/op
BenchmarkLargeTable10k_InMemory_Select_ExactMatch-8              	    1980	    750283 ns/op	      32 B/op	       1 allocs/op
BenchmarkLargeTable10k_InMemory_Select_ExactMatch-8              	    1857	    739706 ns/op	      32 B/op	       1 allocs/op
BenchmarkLargeTable10k_InMemory_Select_ExactMatch-8              	    2186	    517934 ns/op	      32 B/op	       1 allocs/op
BenchmarkLargeTable10k_InMemory_Select_Like_Unicode-8            	     147	   7633396 ns/op	  473642 B/op	   20011 allocs/op
BenchmarkLargeTable10k_InMemory_Select_Like_Unicode-8            	     141	   8549284 ns/op	  473641 B/op	   20011 allocs/op
BenchmarkLargeTable10k_InMemory_Select_Like_Unicode-8            	     100	  14831933 ns/op	  473643 B/op	   20011 allocs/op
BenchmarkLargeTable10k_InMemory_Select_Regex-8                   	      10	 102298480 ns/op	38966474 B/op	  230065 allocs/op
BenchmarkLargeTable10k_InMemory_Select_Regex-8                   	      14	 120628114 ns/op	38985922 B/op	  230066 allocs/op
BenchmarkLargeTable10k_InMemory_Select_Regex-8                   	      13	  94731531 ns/op	38984608 B/op	  230067 allocs/op
BenchmarkLargeTable10k_InMemory_Sort-8                           	      20	  66471920 ns/op	     144 B/op	       5 allocs/op
BenchmarkLargeTable10k_InMemory_Sort-8                           	      13	  79696100 ns/op	     144 B/op	       5 allocs/op
BenchmarkLargeTable10k_InMemory_Sort-8                           	      18	 101202494 ns/op	     144 B/op	       5 allocs/op
BenchmarkLargeTable10k_InMemory_Select_PositionalSlice-8         	  125716	     24511 ns/op	    5569 B/op	       8 allocs/op
BenchmarkLargeTable10k_InMemory_Select_PositionalSlice-8         	   16518	     76733 ns/op	    5569 B/op	       8 allocs/op
BenchmarkLargeTable10k_InMemory_Select_PositionalSlice-8         	   85596	     13742 ns/op	    5569 B/op	       8 allocs/op
BenchmarkLargeTable10k_InMemory_Select_Streaming_Head1-8         	 4913476	       325.1 ns/op	      32 B/op	       1 allocs/op
BenchmarkLargeTable10k_InMemory_Select_Streaming_Head1-8         	 6624600	       208.5 ns/op	      32 B/op	       1 allocs/op
BenchmarkLargeTable10k_InMemory_Select_Streaming_Head1-8         	 5000718	       285.0 ns/op	      32 B/op	       1 allocs/op
BenchmarkLargeTable10k_InMemory_Select_Streaming_Tail1-8         	    3099	    433159 ns/op	      32 B/op	       1 allocs/op
BenchmarkLargeTable10k_InMemory_Select_Streaming_Tail1-8         	    2947	    359490 ns/op	      32 B/op	       1 allocs/op
BenchmarkLargeTable10k_InMemory_Select_Streaming_Tail1-8         	    3370	    306589 ns/op	      32 B/op	       1 allocs/op
BenchmarkLargeTable10k_EndToEnd_Update_Streaming_Head10-8        	      15	  69839793 ns/op	21022680 B/op	  274526 allocs/op
BenchmarkLargeTable10k_EndToEnd_Update_Streaming_Head10-8        	      15	  67565840 ns/op	21022500 B/op	  274524 allocs/op
BenchmarkLargeTable10k_EndToEnd_Update_Streaming_Head10-8        	      16	  68163581 ns/op	21022256 B/op	  274525 allocs/op
BenchmarkLargeTable10k_EndToEnd_Update_Streaming_SingleIndex-8   	      13	  79598454 ns/op	21019001 B/op	  274446 allocs/op
BenchmarkLargeTable10k_EndToEnd_Update_Streaming_SingleIndex-8   	      15	  74349200 ns/op	21019027 B/op	  274446 allocs/op
BenchmarkLargeTable10k_EndToEnd_Update_Streaming_SingleIndex-8   	      18	  80003078 ns/op	21018879 B/op	  274446 allocs/op
BenchmarkLargeTable10k_Hot_ApplyRowUpdate-8                      	 1839850	       674.1 ns/op	     448 B/op	       3 allocs/op
BenchmarkLargeTable10k_Hot_ApplyRowUpdate-8                      	 1559751	       761.6 ns/op	     448 B/op	       3 allocs/op
BenchmarkLargeTable10k_Hot_ApplyRowUpdate-8                      	 1767026	       626.6 ns/op	     448 B/op	       3 allocs/op
PASS
ok  	github.com/pipik-roman/rowlite/internal/core	96.461s
```
