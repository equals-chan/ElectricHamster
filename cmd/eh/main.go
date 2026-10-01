// Command eh is a small CLI harness used to exercise the archive engine
// (milestone M1) before the Wails GUI exists.
package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/equals-chan/ElectricHamster/internal/archive"
	"github.com/equals-chan/ElectricHamster/internal/config"
	"github.com/equals-chan/ElectricHamster/internal/export"
	"github.com/equals-chan/ElectricHamster/internal/id"
	"github.com/equals-chan/ElectricHamster/internal/job"
	"github.com/equals-chan/ElectricHamster/internal/legacy"
	"github.com/equals-chan/ElectricHamster/internal/store"
	"github.com/equals-chan/ElectricHamster/internal/vault"
	"golang.org/x/term"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	ctx := context.Background()
	var err error
	switch os.Args[1] {
	case "info":
		err = runInfo(ctx)
	case "compress", "a":
		err = runCompress(ctx, os.Args[2:])
	case "extract", "x":
		err = runExtract(ctx, os.Args[2:])
	case "list", "l":
		err = runList(ctx, os.Args[2:])
	case "verify", "t":
		err = runVerify(ctx, os.Args[2:])
	case "vault":
		err = runVault(ctx, os.Args[2:])
	case "run":
		err = runJob(ctx, os.Args[2:])
	case "tasks":
		err = listTasks(ctx, os.Args[2:])
	case "archives":
		err = listArchives(ctx, os.Args[2:])
	case "export":
		err = runExport(ctx, os.Args[2:])
	case "import":
		err = runImport(ctx, os.Args[2:])
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

// flagsFirst lets users place flags after positional arguments by moving them
// ahead of the positionals, which the stdlib flag package requires.
func flagsFirst(args []string, boolFlags ...string) []string {
	bools := map[string]bool{}
	for _, b := range boolFlags {
		bools[strings.TrimLeft(b, "-")] = true
	}
	var flags, positional []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			positional = append(positional, args[i+1:]...)
			break
		}
		if len(a) > 0 && a[0] == '-' {
			flags = append(flags, a)
			named := strings.TrimLeft(a, "-")
			if !strings.Contains(a, "=") && !bools[named] && i+1 < len(args) {
				i++
				flags = append(flags, args[i])
			}
			continue
		}
		positional = append(positional, a)
	}
	return append(flags, positional...)
}

func usage() {
	fmt.Fprintln(os.Stderr, strings.TrimSpace(`
ElectricHamster harness

Archive engine (M1):
  eh info
  eh compress <srcDir> <outFile> [--codec zstd|lzma2] [--level N] [--pwd S] [--no-encrypt] [--no-verify] [--split BYTES] [--threads N]
  eh extract  <archive> <outDir> [--pwd S]
  eh list     <archive> [--pwd S]
  eh verify   <archive> [--pwd S]

Vault (M2):
  eh vault init <path> --master S [--time N] [--memory-mib N] [--parallelism N]
  eh vault put  <path> --master S --label L [--password S | --generate N] [--note S]
  eh vault get  <path> --master S <id>
  eh vault list <path>
  eh vault del  <path> --master S <id>
  eh vault changepw <path> --old S --new S
  eh vault gen [--length N] [--symbols]

Pipeline (M3):
  eh run <sourceDir> <destDir> [--name N] [--app-db P] [--vault P] [--master S]
         [--codec zstd|lzma2] [--level N] [--no-encrypt] [--pwd-length N] [--pwd-fixed S]
         [--workers N] [--split BYTES] [--no-verify]
  eh tasks    [--app-db P]
  eh archives [--app-db P] [--task ID]

Excel export (M5):
  eh export <out.xlsx> [--app-db P] [--vault P] [--master S] [--task ID]

Legacy import (M5):
  eh import <config.properties> [--app-db P] [--vault P] [--master S]

Master password may also be supplied via $EH_MASTER, or prompted for.
`))
}

func runInfo(ctx context.Context) error {
	z, err := archive.NewSevenZip(ctx)
	if err != nil {
		return err
	}
	info, _ := z.Info(ctx)
	fmt.Printf("binary  : %s\n", info.Path)
	fmt.Printf("version : %s\n", info.Version)
	fmt.Printf("codecs  : %s\n", strings.Join(info.Codecs, ", "))
	fmt.Printf("zstd=%v aes=%v\n", info.HasZstd, info.HasAES)
	return nil
}

func runCompress(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("compress", flag.ContinueOnError)
	codec := fs.String("codec", "zstd", "zstd|lzma2")
	level := fs.Int("level", -1, "compression level (0-22 zstd, 0-9 lzma2)")
	pwd := fs.String("pwd", "", "password")
	noEncrypt := fs.Bool("no-encrypt", false, "disable encryption")
	noVerify := fs.Bool("no-verify", false, "skip integrity verification")
	split := fs.Int64("split", 0, "volume size in bytes")
	threads := fs.Int("threads", 0, "7-Zip thread count")
	if err := fs.Parse(flagsFirst(args, "no-encrypt", "no-verify")); err != nil {
		return err
	}
	if fs.NArg() < 2 {
		return fmt.Errorf("usage: eh compress <srcDir> <outFile>")
	}
	opt := archive.DefaultOptions()
	opt.Codec = archive.Codec(*codec)
	if *level >= 0 {
		opt.Level = *level
	}
	opt.Password = *pwd
	opt.Encrypt = !*noEncrypt
	opt.Verify = !*noVerify
	opt.SplitSize = *split
	opt.Threads = *threads

	z, err := archive.NewSevenZip(ctx)
	if err != nil {
		return err
	}
	err = z.Compress(ctx, fs.Arg(0), fs.Arg(1), opt, printProgress)
	fmt.Println()
	return err
}

func runExtract(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("extract", flag.ContinueOnError)
	pwd := fs.String("pwd", "", "password")
	if err := fs.Parse(flagsFirst(args)); err != nil {
		return err
	}
	if fs.NArg() < 2 {
		return fmt.Errorf("usage: eh extract <archive> <outDir>")
	}
	z, err := archive.NewSevenZip(ctx)
	if err != nil {
		return err
	}
	err = z.Extract(ctx, fs.Arg(0), fs.Arg(1), *pwd, printProgress)
	fmt.Println()
	return err
}

func runList(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	pwd := fs.String("pwd", "", "password")
	if err := fs.Parse(flagsFirst(args)); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		return fmt.Errorf("usage: eh list <archive>")
	}
	z, err := archive.NewSevenZip(ctx)
	if err != nil {
		return err
	}
	entries, err := z.List(ctx, fs.Arg(0), *pwd)
	if err != nil {
		return err
	}
	for _, e := range entries {
		kind := "f"
		if e.IsDir {
			kind = "d"
		}
		fmt.Printf("%s %12d  %s\n", kind, e.Size, e.Path)
	}
	return nil
}

func runVerify(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("verify", flag.ContinueOnError)
	pwd := fs.String("pwd", "", "password")
	if err := fs.Parse(flagsFirst(args)); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		return fmt.Errorf("usage: eh verify <archive>")
	}
	z, err := archive.NewSevenZip(ctx)
	if err != nil {
		return err
	}
	if err := z.Verify(ctx, fs.Arg(0), *pwd); err != nil {
		return err
	}
	fmt.Println("OK")
	return nil
}

func printProgress(p archive.Progress) {
	if p.Current != "" {
		fmt.Fprintf(os.Stderr, "\r[%-6s] %5.1f%%  %s", p.Phase, p.Percent, truncate(p.Current, 60))
		return
	}
	fmt.Fprintf(os.Stderr, "\r[%-6s] %5.1f%%  out=%d", p.Phase, p.Percent, p.BytesOut)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return "..." + s[len(s)-n+3:]
}

// --- vault subcommands ---

func runVault(ctx context.Context, args []string) error {
	_ = ctx
	if len(args) == 0 {
		return fmt.Errorf("usage: eh vault <init|put|get|list|del|changepw|gen> ...")
	}
	sub := args[0]
	rest := args[1:]
	switch sub {
	case "init":
		return vaultInit(rest)
	case "put":
		return vaultPut(rest)
	case "get":
		return vaultGet(rest)
	case "list":
		return vaultList(rest)
	case "del", "delete":
		return vaultDelete(rest)
	case "changepw":
		return vaultChangePw(rest)
	case "gen", "generate":
		return vaultGen(rest)
	default:
		return fmt.Errorf("unknown vault subcommand %q", sub)
	}
}

func vaultInit(args []string) error {
	fs := flag.NewFlagSet("vault init", flag.ContinueOnError)
	master := fs.String("master", "", "master password")
	t := fs.Int("time", -1, "Argon2id passes")
	memMiB := fs.Int("memory-mib", -1, "Argon2id memory in MiB")
	par := fs.Int("parallelism", -1, "Argon2id parallelism")
	if err := fs.Parse(flagsFirst(args)); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		return fmt.Errorf("usage: eh vault init <path> --master S")
	}
	pass, err := masterPassword(*master)
	if err != nil {
		return err
	}
	params := vault.DefaultKDFParams()
	if *t > 0 {
		params.Time = uint32(*t)
	}
	if *memMiB > 0 {
		params.Memory = uint32(*memMiB) * 1024
	}
	if *par > 0 {
		params.Parallelism = uint8(*par)
	}
	v, err := vault.Create(fs.Arg(0), pass, params)
	if err != nil {
		return err
	}
	defer v.Close()
	fmt.Printf("vault created: %s (argon2id time=%d memory=%dMiB parallelism=%d)\n",
		fs.Arg(0), params.Time, params.Memory/1024, params.Parallelism)
	return nil
}

func vaultPut(args []string) error {
	fs := flag.NewFlagSet("vault put", flag.ContinueOnError)
	master := fs.String("master", "", "master password")
	label := fs.String("label", "", "entry label (e.g. folder name)")
	password := fs.String("password", "", "password to store")
	generate := fs.Int("generate", 0, "generate a random password of this length")
	symbols := fs.Bool("symbols", false, "include symbols when generating")
	note := fs.String("note", "", "optional note")
	if err := fs.Parse(flagsFirst(args, "symbols")); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		return fmt.Errorf("usage: eh vault put <path> --label L [--password S | --generate N]")
	}
	if *label == "" {
		return fmt.Errorf("--label is required")
	}
	pass := *password
	if *generate > 0 {
		charset := vault.CharsetAlnum
		if *symbols {
			charset = vault.CharsetSymbols
		}
		generated, err := vault.GeneratePasswordWith(*generate, charset)
		if err != nil {
			return err
		}
		pass = generated
	} else if pass == "" {
		prompted, err := readSecret("Entry password: ")
		if err != nil {
			return err
		}
		pass = prompted
	}

	v, err := vault.Open(fs.Arg(0))
	if err != nil {
		return err
	}
	defer v.Close()
	masterPass, err := masterPassword(*master)
	if err != nil {
		return err
	}
	if err := v.Unlock(masterPass); err != nil {
		return err
	}
	id, err := v.Put(*label, pass, *note)
	if err != nil {
		return err
	}
	fmt.Printf("stored %s\n", id)
	if *generate > 0 {
		fmt.Printf("generated password: %s\n", pass)
	}
	return nil
}

func vaultGet(args []string) error {
	fs := flag.NewFlagSet("vault get", flag.ContinueOnError)
	master := fs.String("master", "", "master password")
	if err := fs.Parse(flagsFirst(args)); err != nil {
		return err
	}
	if fs.NArg() < 2 {
		return fmt.Errorf("usage: eh vault get <path> <id>")
	}
	v, err := vault.Open(fs.Arg(0))
	if err != nil {
		return err
	}
	defer v.Close()
	masterPass, err := masterPassword(*master)
	if err != nil {
		return err
	}
	if err := v.Unlock(masterPass); err != nil {
		return err
	}
	item, err := v.Get(fs.Arg(1))
	if err != nil {
		return err
	}
	fmt.Printf("label   : %s\n", item.Label)
	fmt.Printf("password: %s\n", item.Password)
	if item.Note != "" {
		fmt.Printf("note    : %s\n", item.Note)
	}
	return nil
}

func vaultList(args []string) error {
	fs := flag.NewFlagSet("vault list", flag.ContinueOnError)
	if err := fs.Parse(flagsFirst(args)); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		return fmt.Errorf("usage: eh vault list <path>")
	}
	v, err := vault.Open(fs.Arg(0))
	if err != nil {
		return err
	}
	defer v.Close()
	items, err := v.List()
	if err != nil {
		return err
	}
	for _, it := range items {
		fmt.Printf("%s  %-30s  %s\n", it.ID, it.Label, it.CreatedAt.Format(time.RFC3339))
	}
	fmt.Printf("total: %d\n", len(items))
	return nil
}

func vaultDelete(args []string) error {
	fs := flag.NewFlagSet("vault del", flag.ContinueOnError)
	master := fs.String("master", "", "master password")
	if err := fs.Parse(flagsFirst(args)); err != nil {
		return err
	}
	if fs.NArg() < 2 {
		return fmt.Errorf("usage: eh vault del <path> <id>")
	}
	v, err := vault.Open(fs.Arg(0))
	if err != nil {
		return err
	}
	defer v.Close()
	masterPass, err := masterPassword(*master)
	if err != nil {
		return err
	}
	if err := v.Unlock(masterPass); err != nil {
		return err
	}
	if err := v.Delete(fs.Arg(1)); err != nil {
		return err
	}
	fmt.Println("deleted")
	return nil
}

func vaultChangePw(args []string) error {
	fs := flag.NewFlagSet("vault changepw", flag.ContinueOnError)
	oldPw := fs.String("old", "", "current master password")
	newPw := fs.String("new", "", "new master password")
	if err := fs.Parse(flagsFirst(args)); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		return fmt.Errorf("usage: eh vault changepw <path> --old S --new S")
	}
	if *newPw == "" {
		return fmt.Errorf("--new is required")
	}
	v, err := vault.Open(fs.Arg(0))
	if err != nil {
		return err
	}
	defer v.Close()
	if err := v.ChangeMasterPassword(*oldPw, *newPw); err != nil {
		return err
	}
	fmt.Println("master password changed")
	return nil
}

func vaultGen(args []string) error {
	fs := flag.NewFlagSet("vault gen", flag.ContinueOnError)
	length := fs.Int("length", 20, "password length")
	symbols := fs.Bool("symbols", false, "include symbols")
	if err := fs.Parse(flagsFirst(args, "symbols")); err != nil {
		return err
	}
	charset := vault.CharsetAlnum
	if *symbols {
		charset = vault.CharsetSymbols
	}
	p, err := vault.GeneratePasswordWith(*length, charset)
	if err != nil {
		return err
	}
	fmt.Println(p)
	return nil
}

func masterPassword(flagVal string) (string, error) {
	if flagVal != "" {
		return flagVal, nil
	}
	if v := os.Getenv("EH_MASTER"); v != "" {
		return v, nil
	}
	return readSecret("Master password: ")
}

func readSecret(prompt string) (string, error) {
	fd := int(os.Stdin.Fd())
	if term.IsTerminal(fd) {
		fmt.Fprint(os.Stderr, prompt)
		b, err := term.ReadPassword(fd)
		fmt.Fprintln(os.Stderr)
		return string(b), err
	}
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && line == "" {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}

// --- pipeline ---

type eventPrinter struct{ mu sync.Mutex }

func (p *eventPrinter) emit(e job.Event) {
	p.mu.Lock()
	defer p.mu.Unlock()
	switch e.Kind {
	case job.EventRunStarted:
		fmt.Printf("run %s: %d folder(s)\n", shortID(e.RunID), e.Total)
	case job.EventItemStarted:
		fmt.Printf("  [%d/%d] %s ...\n", e.Index, e.Total, e.Folder)
	case job.EventItemSkipped:
		if e.ArchivePath != "" {
			fmt.Printf("  - %s (already archived: %s)\n", e.Folder, e.ArchivePath)
		} else {
			fmt.Printf("  - %s (skipped)\n", e.Folder)
		}
	case job.EventItemDone:
		if e.Err != nil {
			fmt.Printf("  ! %s: %v\n", e.Folder, e.Err)
		} else {
			fmt.Printf("  + %s -> %s\n", e.Folder, e.ArchivePath)
		}
	case job.EventRunFinished:
		s := e.Stats
		fmt.Printf("run %s finished: done=%d skipped=%d failed=%d in=%d out=%d in %s\n",
			shortID(e.RunID), s.Done, s.Skipped, s.Failed, s.BytesIn, s.BytesOut,
			s.Elapsed.Round(time.Millisecond))
	}
}

func shortID(s string) string {
	if len(s) > 8 {
		return s[:8]
	}
	return s
}

func runJob(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	appDB := fs.String("app-db", "", "application database path")
	vaultPath := fs.String("vault", "", "vault path")
	master := fs.String("master", "", "master password")
	name := fs.String("name", "", "task name (default: source folder name)")
	codec := fs.String("codec", "zstd", "zstd|lzma2")
	level := fs.Int("level", 15, "compression level")
	noEncrypt := fs.Bool("no-encrypt", false, "disable encryption")
	pwdLength := fs.Int("pwd-length", 20, "random password length")
	pwdFixed := fs.String("pwd-fixed", "", "fixed password for every archive")
	workers := fs.Int("workers", 0, "worker count (default: NumCPU)")
	split := fs.Int64("split", 0, "volume size in bytes")
	dedup := fs.String("dedup", "content_sig", "dedup rule: content_sig|folder_name|none")
	prefix := fs.String("prefix", "", "archive name prefix (e.g. game_)")
	suffix := fs.String("suffix", "", "archive name suffix")
	seqStart := fs.Int64("seq-start", 0, "first archive number (default 10000)")
	noVerify := fs.Bool("no-verify", false, "skip post-compression verification")
	if err := fs.Parse(flagsFirst(args, "no-encrypt", "no-verify")); err != nil {
		return err
	}
	if fs.NArg() < 2 {
		return fmt.Errorf("usage: eh run <sourceDir> <destDir> [flags]")
	}
	sourceDir, err := filepath.Abs(fs.Arg(0))
	if err != nil {
		return err
	}
	destDir, err := filepath.Abs(fs.Arg(1))
	if err != nil {
		return err
	}
	if *name == "" {
		*name = filepath.Base(sourceDir)
	}
	if *appDB == "" {
		p, err := config.DefaultAppDB()
		if err != nil {
			return err
		}
		*appDB = p
	}
	if *vaultPath == "" {
		p, err := config.DefaultVault()
		if err != nil {
			return err
		}
		*vaultPath = p
	}

	st, err := store.Open(*appDB)
	if err != nil {
		return err
	}
	defer st.Close()

	task, isNew, err := findOrInitTask(ctx, st, *name)
	if err != nil {
		return err
	}
	task.SourceDir = sourceDir
	task.DestDir = destDir
	task.Format = "7z"
	task.Codec = *codec
	task.Level = *level
	task.Encrypt = !*noEncrypt
	task.HeaderEncrypt = !*noEncrypt
	task.SplitSize = *split
	task.DedupRule = *dedup
	task.NamePrefix = *prefix
	task.NameSuffix = *suffix
	if *seqStart > 0 {
		task.SeqStart = *seqStart
	}
	task.Enabled = true
	switch {
	case *noEncrypt:
		task.Password = store.PasswordPolicy{Mode: "none"}
	case *pwdFixed != "":
		task.Password = store.PasswordPolicy{Mode: "fixed", Fixed: *pwdFixed}
	default:
		task.Password = store.PasswordPolicy{Mode: "random", Length: *pwdLength}
	}
	if isNew {
		if err := st.CreateTask(ctx, task); err != nil {
			return err
		}
	} else if err := st.UpdateTask(ctx, task); err != nil {
		return err
	}

	var v *vault.Vault
	if task.Encrypt {
		pass, err := masterPassword(*master)
		if err != nil {
			return err
		}
		if _, statErr := os.Stat(*vaultPath); errors.Is(statErr, os.ErrNotExist) {
			v, err = vault.Create(*vaultPath, pass, vault.DefaultKDFParams())
		} else {
			v, err = vault.Open(*vaultPath)
			if err == nil {
				err = v.Unlock(pass)
			}
		}
		if err != nil {
			return err
		}
		defer v.Close()
	}

	z, err := archive.NewSevenZip(ctx)
	if err != nil {
		return err
	}
	engine := &job.Engine{Store: st, Vault: v, Archiver: z, Workers: *workers, Verify: !*noVerify}
	printer := &eventPrinter{}
	if _, err := engine.Run(ctx, task, printer.emit); err != nil {
		return err
	}
	return nil
}

func findOrInitTask(ctx context.Context, st *store.Store, name string) (store.Task, bool, error) {
	task, err := st.FindTaskByName(ctx, name)
	if err == nil {
		return task, false, nil
	}
	if !errors.Is(err, store.ErrNotFound) {
		return store.Task{}, false, err
	}
	return store.Task{ID: id.New(), Name: name, DedupRule: "content_sig"}, true, nil
}

func listTasks(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("tasks", flag.ContinueOnError)
	appDB := fs.String("app-db", "", "application database path")
	if err := fs.Parse(flagsFirst(args)); err != nil {
		return err
	}
	if *appDB == "" {
		p, err := config.DefaultAppDB()
		if err != nil {
			return err
		}
		*appDB = p
	}
	st, err := store.Open(*appDB)
	if err != nil {
		return err
	}
	defer st.Close()
	tasks, err := st.ListTasks(ctx)
	if err != nil {
		return err
	}
	for _, t := range tasks {
		fmt.Printf("%s  %-18s %s -> %s  [%s lvl%d enc=%v policy=%s dedup=%s name=%s{seq}%s start=%d]\n",
			shortID(t.ID), t.Name, t.SourceDir, t.DestDir, t.Codec, t.Level, t.Encrypt, t.Password.Mode,
			t.DedupRule, t.NamePrefix, t.NameSuffix, t.SeqStart)
	}
	fmt.Printf("total: %d\n", len(tasks))
	return nil
}

func listArchives(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("archives", flag.ContinueOnError)
	appDB := fs.String("app-db", "", "application database path")
	taskID := fs.String("task", "", "filter by task id")
	if err := fs.Parse(flagsFirst(args)); err != nil {
		return err
	}
	if *appDB == "" {
		p, err := config.DefaultAppDB()
		if err != nil {
			return err
		}
		*appDB = p
	}
	st, err := store.Open(*appDB)
	if err != nil {
		return err
	}
	defer st.Close()
	archives, err := st.ListArchives(ctx, *taskID)
	if err != nil {
		return err
	}
	for _, a := range archives {
		fmt.Printf("%6d  %-18s %10d  %s  pwd=%s\n",
			a.Seq, a.FolderName, a.ArchiveSize, a.ArchivePath, shortID(a.PasswordID))
	}
	fmt.Printf("total: %d\n", len(archives))
	return nil
}

func runExport(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("export", flag.ContinueOnError)
	appDB := fs.String("app-db", "", "application database path")
	vaultPath := fs.String("vault", "", "vault path")
	master := fs.String("master", "", "master password")
	taskID := fs.String("task", "", "filter by task id")
	if err := fs.Parse(flagsFirst(args)); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		return fmt.Errorf("usage: eh export <out.xlsx> [--app-db P] [--vault P] [--master S] [--task ID]")
	}
	out := fs.Arg(0)
	if *appDB == "" {
		p, err := config.DefaultAppDB()
		if err != nil {
			return err
		}
		*appDB = p
	}
	if *vaultPath == "" {
		p, err := config.DefaultVault()
		if err != nil {
			return err
		}
		*vaultPath = p
	}

	st, err := store.Open(*appDB)
	if err != nil {
		return err
	}
	defer st.Close()
	archives, err := st.ListArchives(ctx, *taskID)
	if err != nil {
		return err
	}

	var v *vault.Vault
	if _, statErr := os.Stat(*vaultPath); statErr == nil {
		v, err = vault.Open(*vaultPath)
		if err != nil {
			return err
		}
		defer v.Close()
		pass, err := masterPassword(*master)
		if err != nil {
			return err
		}
		if err := v.Unlock(pass); err != nil {
			return err
		}
	}

	rows := make([]export.ArchiveRow, 0, len(archives))
	for _, a := range archives {
		pw := ""
		if a.PasswordID != "" {
			if v == nil {
				return fmt.Errorf("archive %d has a stored password but no vault was found", a.Seq)
			}
			item, err := v.Get(a.PasswordID)
			if err != nil {
				return err
			}
			pw = item.Password
		}
		rows = append(rows, export.ArchiveRow{
			Seq:         a.Seq,
			FolderName:  a.FolderName,
			ArchivePath: a.ArchivePath,
			Password:    pw,
			Size:        a.ArchiveSize,
			FileCount:   a.FileCount,
			CreatedAt:   a.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}
	if err := export.WriteArchives(out, rows); err != nil {
		return err
	}
	fmt.Printf("exported %d rows to %s\n", len(rows), out)
	return nil
}

func runImport(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("import", flag.ContinueOnError)
	appDB := fs.String("app-db", "", "application database path")
	vaultPath := fs.String("vault", "", "vault path")
	master := fs.String("master", "", "master password")
	if err := fs.Parse(flagsFirst(args)); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		return fmt.Errorf("usage: eh import <config.properties> [--app-db P] [--vault P] [--master S]")
	}
	cfgPath := fs.Arg(0)
	if *appDB == "" {
		p, err := config.DefaultAppDB()
		if err != nil {
			return err
		}
		*appDB = p
	}
	if *vaultPath == "" {
		p, err := config.DefaultVault()
		if err != nil {
			return err
		}
		*vaultPath = p
	}

	st, err := store.Open(*appDB)
	if err != nil {
		return err
	}
	defer st.Close()

	pass, err := masterPassword(*master)
	if err != nil {
		return err
	}
	var v *vault.Vault
	if _, statErr := os.Stat(*vaultPath); errors.Is(statErr, os.ErrNotExist) {
		v, err = vault.Create(*vaultPath, pass, vault.DefaultKDFParams())
	} else {
		v, err = vault.Open(*vaultPath)
		if err == nil {
			err = v.Unlock(pass)
		}
	}
	if err != nil {
		return err
	}
	defer v.Close()

	res, err := legacy.Import(ctx, legacy.ImportOptions{ConfigPath: cfgPath, Store: st, Vault: v})
	if err != nil {
		return err
	}
	fmt.Printf("imported: task=%s created=%v archives=%d passwords=%d skipped=%d\n",
		shortID(res.TaskID), res.TaskCreated, res.Archives, res.PasswordsIn, res.Skipped)
	return nil
}
