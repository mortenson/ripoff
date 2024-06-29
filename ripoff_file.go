package ripoff

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"text/template"

	"gopkg.in/yaml.v3"
)

type Row map[string]string

type RipoffFile struct {
	Rows map[string]Row `yaml:"rows"`
}

var funcMap = template.FuncMap{
	// Convenient way to loop a set amount of times.
	"intSlice": func(countStr string) ([]int, error) {
		countInt, err := strconv.Atoi(countStr)
		if err != nil {
			return []int{}, err
		}
		ret := make([]int, countInt)
		for i := range ret {
			ret[i] = i
		}
		return ret, nil
	},
}

var templateFileRegex = regexp.MustCompile(`^template_(\S+)\.`)

// Adds newRows to existingRows, processing templated rows when needed.
func concatRows(templates *template.Template, existingRows map[string]Row, newRows map[string]Row) error {
	for rowId, row := range newRows {
		_, rowExists := existingRows[rowId]
		if rowExists {
			return fmt.Errorf("row %s is defined more than once", rowId)
		}
		templateName, usesTemplate := row["template"]
		if usesTemplate {
			// "rowId" allows dependencies between templated rows to be clear outside of the template.
			// Templates can additionally use it to seed random generators.
			templateVars := row
			templateVars["rowId"] = rowId
			buf := &bytes.Buffer{}
			err := templates.ExecuteTemplate(buf, templateName, templateVars)
			if err != nil {
				return err
			}
			ripoff := &RipoffFile{}
			err = yaml.Unmarshal(buf.Bytes(), ripoff)
			if err != nil {
				return err
			}
			for templateRowId, templateRow := range ripoff.Rows {
				_, rowExists := existingRows[templateRowId]
				if rowExists {
					return fmt.Errorf("row %s is defined more than once", rowId)
				}
				existingRows[templateRowId] = templateRow
			}
		} else {
			existingRows[rowId] = row
		}
	}
	return nil
}

// Builds a single RipoffFile from a directory of yaml files.
func RipoffFromDirectory(dir string) (RipoffFile, error) {
	dir = filepath.Clean(dir)

	// Treat files starting with template_ as go templates.
	templates := template.New("").Option("missingkey=error").Funcs(funcMap)
	_, err := templates.ParseGlob(filepath.Join(dir, "template_*"))
	if err != nil && !strings.Contains(err.Error(), "template: pattern matches no files") {
		return RipoffFile{}, err
	}

	// Find all ripoff files in dir recursively.
	allRipoffs := []RipoffFile{}
	err = filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if filepath.Ext(path) != ".yaml" && filepath.Ext(path) != ".yml" {
			return nil
		}
		// Templates were already processed.
		templateNameMatches := templateFileRegex.FindStringSubmatch(entry.Name())
		if len(templateNameMatches) == 2 {
			return nil
		}

		yamlFile, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		ripoff := &RipoffFile{}
		err = yaml.Unmarshal(yamlFile, ripoff)
		if err != nil {
			return err
		}
		allRipoffs = append(allRipoffs, *ripoff)
		return nil
	})

	if err != nil {
		return RipoffFile{}, err
	}

	totalRipoff := RipoffFile{
		Rows: map[string]Row{},
	}

	for _, ripoff := range allRipoffs {
		err = concatRows(templates, totalRipoff.Rows, ripoff.Rows)
		if err != nil {
			return RipoffFile{}, err
		}
	}

	return totalRipoff, nil
}
