package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
)

type Column struct {
	Name string
}
type Row map[string]any
type Rows []Row

var filterAnd = map[string]bool{
	"<":  true,
	"<=": true,
	">":  true,
	">=": true,
	"!=": true,
}
var filters map[string]func(any, string) bool

func init() {
	filters = make(map[string]func(any, string) bool)
	filters["<"] = func(value any, d string) bool {
		switch v := value.(type) {
		case int:
			i, err := strconv.Atoi(d)
			if err != nil {
				return false
			}
			return i < v
		case int64:
			i, err := strconv.ParseInt(d, 10, 64)
			if err != nil {
				return false
			}
			return i < v
		case float64:
			i, err := strconv.ParseFloat(d, 64)
			if err != nil {
				return false
			}
			return i < v
		case string:
			return d < v
		case nil:
			return false
		default:
			log.Printf("unexpected type: %T", v)
			return false
		}
	}
	filters[">"] = func(value any, d string) bool {
		switch v := value.(type) {
		case int:
			i, err := strconv.Atoi(d)
			if err != nil {
				return false
			}
			return i > v
		case int64:
			i, err := strconv.ParseInt(d, 10, 64)
			if err != nil {
				return false
			}
			return i > v
		case float64:
			i, err := strconv.ParseFloat(d, 64)
			if err != nil {
				return false
			}
			return i > v
		case string:
			return d > v
		case nil:
			return false
		default:
			log.Printf("unexpected type: %T", v)
			return false
		}
	}
	filters["=="] = func(value any, d string) bool {
		switch v := value.(type) {
		case int:
			i, err := strconv.Atoi(d)
			if err != nil {
				return false
			}
			return i == v
		case int64:
			i, err := strconv.ParseInt(d, 10, 64)
			if err != nil {
				return false
			}
			return i == v
		case float64:
			i, err := strconv.ParseFloat(d, 64)
			if err != nil {
				return false
			}
			return math.Abs(i-v) <= 1e-9
		case string:
			return d == v
		case nil:
			return false
		default:
			log.Printf("unexpected type: %T", v)
			return false
		}
	}
	filters["!="] = func(value any, d string) bool {
		switch v := value.(type) {
		case int:
			i, err := strconv.Atoi(d)
			if err != nil {
				return false
			}
			return i != v
		case int64:
			i, err := strconv.ParseInt(d, 10, 64)
			if err != nil {
				return false
			}
			return i != v
		case float64:
			i, err := strconv.ParseFloat(d, 64)
			if err != nil {
				return false
			}
			return math.Abs(i-v) > 1e-9
		case string:
			return d != v
		case nil:
			return false
		default:
			log.Printf("unexpected type: %T", v)
			return false
		}
	}
	filters[">="] = func(value any, d string) bool {
		return filters[">"](value, d) || filters["=="](value, d)
	}
	filters["<="] = func(value any, d string) bool {
		return filters["<"](value, d) || filters["=="](value, d)
	}
}

type Table struct {
	filepath    string         `json:"-"`
	columnIndex map[string]int `json:"-"`
	Name        string         `json:"-"`

	Columns []Column
	Data    [][]string
}

func NewTable(dir, name string) *Table {
	return &Table{
		filepath: filepath.Join(
			dir,
			"tables",
			url.QueryEscape(name)+".json",
		),
		columnIndex: map[string]int{},
		Name:        name,
	}
}

func ReadTable(dir, name string) (*Table, error) {
	table := NewTable(dir, name)

	f, err := os.Open(table.filepath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return table, nil
		}
		return nil, err
	}
	defer f.Close()

	dec := json.NewDecoder(f)
	if err := dec.Decode(table); err != nil {
		return nil, err
	}

	for i, c := range table.Columns {
		table.columnIndex[c.Name] = i
	}

	return table, nil
}

func (t Table) Save() error {
	os.Mkdir(filepath.Dir(t.filepath), 0777)
	f, err := os.OpenFile(t.filepath, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0666)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("  ", "")
	return enc.Encode(t)
}

func (t Table) Get(i int) Row {
	result := Row{}
	result["id"] = i
	for j, v := range t.Data[i] {
		result[t.Columns[j].Name] = v
	}
	return result
}

func (t Table) First() Row {
	return t.Get(0)
}

func (r Rows) First() Row {
	return r[0]
}

func (r Rows) maybeMapReduceColumn(
	column string,
	m func(string) (any, bool),
	reduce func(any, any) any,
	init any,
) any {
	acc := init
	for _, row := range r {
		val, ok := row[column]
		if !ok {
			continue // column not found
		}
		strVal, ok := val.(string)
		if !ok {
			continue // column value not a string
		}
		mapped, ok := m(strVal)
		if !ok {
			continue // mapper rejected value
		}
		acc = reduce(acc, mapped)
	}
	return acc
}

func (r Rows) Min(column string) float64 {
	return r.maybeMapReduceColumn(column, func(s string) (any, bool) {
		f, err := strconv.ParseFloat(s, 64)
		return f, err == nil
	}, func(x, y any) any { return math.Min(x.(float64), y.(float64)) }, math.Inf(1)).(float64)
}

func (r Rows) Max(column string) float64 {
	return r.maybeMapReduceColumn(column, func(s string) (any, bool) {
		f, err := strconv.ParseFloat(s, 64)
		return f, err == nil
	}, func(x, y any) any { return math.Max(x.(float64), y.(float64)) }, math.Inf(-1)).(float64)
}

func (t Table) col(column string) int {
	i, ok := t.columnIndex[column]
	if !ok {
		panic(fmt.Errorf("table %q: column %q: not found", t.Name, column))
	}
	return i
}

func (t Table) Where(column, op string, values ...any) Rows {
	filter, ok := filters[op]
	if !ok {
		panic(fmt.Errorf("unknown binary operator %q", op))
	}
	i := t.col(column)
	rows := Rows{}
	for j, r := range t.Data {
		ok := !filterAnd[op]
		for _, v := range values {
			result := filter(v, r[i])
			if filterAnd[op] {
				ok = ok && result
				if !ok {
					break
				}
			} else {
				ok = ok || result
				if ok {
					break
				}
			}
		}
		if ok {
			rows = append(rows, t.Get(j))
		}
	}
	return rows
}

func (t *Table) Insert(value map[any]any) *Table {
	r := make([]string, len(t.Columns))
	for k, v := range value {
		r[t.col(fmt.Sprint(k))] = fmt.Sprint(v)
	}
	return t
}

// vim: ai:ts=8:sw=8:noet:syntax=go
