package main

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
)

func cmdInit(input io.Reader, output io.Writer, args []string) error {
	switch len(args) {
	case 0:
		_, err := CreateRepository(".")
		return err
	case 1:
		_, err := CreateRepository(args[0])
		return err
	default:
		return errors.New("usage: init [<dir>]")
	}
}

func cmdHashObject(input io.Reader, output io.Writer, args []string) error {
	if len(args) != 2 {
		return errors.New("usage: hash-object <kind> <path>")
	}
	switch args[0] {
	case "commit", "tree", "tag", "blob":
		// All good.
	default:
		return fmt.Errorf("invalid object type")
	}
	repo, err := FindRepository(".")
	if err != nil {
		return fmt.Errorf("cannot open git repository: %w", err)
	}
	content, err := ioutil.ReadFile(args[1])
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}
	if sha, err := repo.WriteObject(args[0], content); err != nil {
		return fmt.Errorf("write object: %w", err)
	} else {
		fmt.Println(hex.EncodeToString(sha))
	}
	return nil
}

func cmdCatFile(input io.Reader, output io.Writer, args []string) error {
	if len(args) != 1 {
		return errors.New("usage: cat-file <sha>")
	}
	hash := args[0]
	sha, err := hex.DecodeString(hash)
	if err != nil {
		return fmt.Errorf("invalid hash value: %w", err)
	}
	repo, err := FindRepository(".")
	if err != nil {
		return fmt.Errorf("cannot open git repository: %w", err)
	}
	obj, err := repo.ReadObject(sha)
	if err != nil {
		return fmt.Errorf("cannot read object: %w", err)
	}
	fmt.Print(obj)
	return nil
}

func cmdLog(input io.Reader, output io.Writer, args []string) error {
	if len(args) != 1 {
		return errors.New("usage: log <sha>")
	}
	hash := args[0]
	sha, err := hex.DecodeString(hash)
	if err != nil {
		return fmt.Errorf("invalid hash value: %w", err)
	}
	repo, err := FindRepository(".")
	if err != nil {
		return fmt.Errorf("cannot open git repository: %w", err)
	}

	var b bytes.Buffer
	fmt.Fprintln(&b, "digraph gogitlog{")
	seen := map[string]struct{}{}
	if err := writeGraphviz(&b, repo, seen, sha); err != nil {
		return err
	}
	fmt.Fprintln(&b, "}")
	_, err = b.WriteTo(output)
	return err
}

func writeGraphviz(w io.Writer, repo *Repository, seen map[string]struct{}, sha []byte) error {
	obj, err := repo.ReadObject(sha)
	if err != nil {
		return fmt.Errorf("read %q object: %w", sha, err)
	}
	c, ok := obj.(*CommitObject)
	if !ok {
		return fmt.Errorf("not a commit object: %T", obj)
	}

	for _, parent := range c.Header["parent"] {
		fmt.Fprintf(w, "\"%x\" -> \"%s\";\n", sha, parent)
		parentSha, err := hex.DecodeString(parent)
		if err != nil {
			return fmt.Errorf("invalid %q parent sha: %w", parent, err)
		}
		if err := writeGraphviz(w, repo, seen, parentSha); err != nil {
			return err
		}
	}
	return nil
}

func cmdLsTree(input io.Reader, output io.Writer, args []string) error {
	if len(args) != 1 {
		return errors.New("usage: ls-tree <sha>")
	}
	hash := args[0]
	sha, err := hex.DecodeString(hash)
	if err != nil {
		return fmt.Errorf("invalid hash value: %w", err)
	}
	repo, err := FindRepository(".")
	if err != nil {
		return fmt.Errorf("cannot open git repository: %w", err)
	}

	obj, err := repo.ReadObject(sha)
	if err != nil {
		return fmt.Errorf("read %q object: %w", sha, err)
	}
	tr, ok := obj.(*TreeObject)
	if !ok {
		return fmt.Errorf("not a tree object: %T", obj)
	}
	for _, leaf := range tr.Leafs {
		fmt.Printf("%s\t%q\t%x\n", leaf.Mode, leaf.Path, leaf.Sha)
	}
	return nil

}

func cmdCheckout(input io.Reader, output io.Writer, args []string) error {
	if len(args) != 2 {
		return errors.New("usage: checkout <commit> <path>")
	}

	sha, err := hex.DecodeString(args[0])
	if err != nil {
		return fmt.Errorf("invalid hash value: %w", err)
	}
	repo, err := FindRepository(".")
	if err != nil {
		return fmt.Errorf("cannot open git repository: %w", err)
	}

	obj, err := repo.ReadObject(sha)
	if err != nil {
		return fmt.Errorf("read %q object: %w", sha, err)
	}

	var tr *TreeObject
	switch obj := obj.(type) {
	case *CommitObject:
		sha, err := hex.DecodeString(obj.Header["tree"][0]) // Can not exist?
		if err != nil {
			return fmt.Errorf("invalid tree hash value: %w", err)
		}
		tobj, err := repo.ReadObject(sha)
		if err != nil {
			return fmt.Errorf("read %q tree object: %w", sha, err)
		}
		tr = tobj.(*TreeObject)
	case *TreeObject:
		tr = obj
	default:
		return fmt.Errorf("unexpected %T", obj)
	}

	destDir, err := filepath.Abs(args[1])
	if err != nil {
		return fmt.Errorf("absolute path for %q: %w", args[1], err)
	}

	_ = os.MkdirAll(destDir, newDirPerm)

	// Use path instead of repo path to allow to checkout in any directory.
	// This is better for testing.
	return treeCheckout(repo, tr, destDir)
}

func treeCheckout(repo *Repository, tr *TreeObject, path string) error {
	for _, leaf := range tr.Leafs {
		obj, err := repo.ReadObject(leaf.Sha)
		if err != nil {
			return fmt.Errorf("read %x: %w", leaf.Sha, err)
		}
		dest := filepath.Join(path, leaf.Path)
		switch obj := obj.(type) {
		case *TreeObject:
			if err := os.MkdirAll(dest, newDirPerm); err != nil {
				return fmt.Errorf("mkdir %q: %w", dest, err)
			}
			treeCheckout(repo, obj, dest)
		case *BlobObject:
			if err := ioutil.WriteFile(dest, obj.Data, 0644); err != nil {
				return fmt.Errorf("write %q blob: %w", dest, err)
			}
		default:
			return fmt.Errorf("unexpected %T", obj)
		}
	}
	return nil
}

func cmdShowRef(input io.Reader, output io.Writer, args []string) error {
	if len(args) != 0 {
		return errors.New("usage: show-ref")
	}

	repo, err := FindRepository(".")
	if err != nil {
		return fmt.Errorf("cannot open git repository: %w", err)
	}

	var b bytes.Buffer
	err = filepath.Walk(filepath.Join(repo.gitdir, "refs"), func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		content, err := ioutil.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %q: %w", path, err)
		}
		ref := string(bytes.TrimSpace(content))
		if _, err := fmt.Fprintf(&b, "%s %s\n", ref, path[len(repo.gitdir)+1:]); err != nil {
			return fmt.Errorf("write: %w", err)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("walk: %w", err)
	}
	if _, err := b.WriteTo(output); err != nil {
		return fmt.Errorf("write to stdout: %w", err)
	}
	return nil
}

func cmdTag(input io.Reader, output io.Writer, args []string) error {
	if len(args) != 2 {
		// Only a single format is supported. Lazy.
		return errors.New("usage: tag <name> <hash>")
	}

	repo, err := FindRepository(".")
	if err != nil {
		return fmt.Errorf("cannot open git repository: %w", err)
	}

	if err := repo.WriteFile(true, []byte(args[1]+"\n"), "refs", "tags", args[0]); err != nil {
		return fmt.Errorf("write tag file: %w", err)
	}
	return nil
}
