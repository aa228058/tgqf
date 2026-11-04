package messages

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

type xlsxSharedString struct {
	Text string `xml:"t"`
	Runs []struct { Text string `xml:"t"` } `xml:"r"`
}

type xlsxSharedStrings struct {
	Items []xlsxSharedString `xml:"si"`
}

type xlsxCell struct {
	Ref string `xml:"r,attr"`
	Type string `xml:"t,attr"`
	Value string `xml:"v"`
	Inline struct { Text string `xml:"t"` } `xml:"is"`
}

type xlsxWorksheet struct {
	SheetData struct {
		Rows []struct { Cells []xlsxCell `xml:"c"` } `xml:"row"`
	} `xml:"sheetData"`
}

func xlsxColumnIndex(ref string) int {
	n := 0
	for _, r := range ref {
		if r < 'A' || r > 'Z' { break }
		n = n*26 + int(r-'A'+1)
	}
	return n-1
}

func parseGroupCatalogueXLSX(data []byte) ([]GroupCatalogueImportRecord, error) {
	if len(data)==0 { return nil, errors.New("XLSX file is empty") }
	reader,err:=zip.NewReader(bytes.NewReader(data),int64(len(data)))
	if err!=nil { return nil,errors.New("invalid XLSX ZIP container") }
	files:=map[string]*zip.File{}
	for _,file:=range reader.File { files[file.Name]=file }
	readFile:=func(name string)([]byte,error){
		file:=files[name];if file==nil{return nil,fmt.Errorf("XLSX is missing %s",name)}
		rc,e:=file.Open();if e!=nil{return nil,e};defer rc.Close()
		return io.ReadAll(io.LimitReader(rc,8<<20))
	}
	shared:=[]string{}
	if files["xl/sharedStrings.xml"]!=nil {
		raw,e:=readFile("xl/sharedStrings.xml");if e!=nil{return nil,e}
		var table xlsxSharedStrings
		if e=xml.Unmarshal(raw,&table);e!=nil{return nil,errors.New("invalid XLSX shared strings")}
		for _,item:=range table.Items {
			value:=item.Text
			if len(item.Runs)>0 { value="";for _,run:=range item.Runs { value+=run.Text } }
			shared=append(shared,value)
		}
	}
	// The first worksheet is the import sheet; workbook relationships are not
	// needed for files produced by Excel/LibreOffice using the conventional path.
	raw,err:=readFile("xl/worksheets/sheet1.xml")
	if err!=nil{return nil,err}
	var sheet xlsxWorksheet
	if err=xml.Unmarshal(raw,&sheet);err!=nil{return nil,errors.New("invalid XLSX worksheet")}
	rows:=make([][]string,0,len(sheet.SheetData.Rows))
	for _,row:=range sheet.SheetData.Rows {
		values:=map[int]string{};maxCol:=-1
		for _,cell:=range row.Cells {
			col:=xlsxColumnIndex(cell.Ref);if col<0{continue}
			value:=cell.Value
			switch cell.Type {
			case "s":
				index,e:=strconv.Atoi(value);if e!=nil||index<0||index>=len(shared){return nil,errors.New("invalid XLSX shared string index")}
				value=shared[index]
			case "inlineStr":
				value=cell.Inline.Text
			case "b":
				if value=="1"{value="true"}else{value="false"}
			}
			values[col]=value;if col>maxCol{maxCol=col}
		}
		if maxCol<0 { continue }
		valuesRow:=make([]string,maxCol+1)
		for col,value:=range values { valuesRow[col]=value }
		rows=append(rows,valuesRow)
	}
	if len(rows)==0{return nil,errors.New("XLSX worksheet contains no rows")}
	var tab bytes.Buffer
	tabWriter:=csv.NewWriter(&tab)
	tabWriter.Comma='\t'
	for _,row:=range rows {
		for i,value:=range row { row[i]=strings.ReplaceAll(strings.ReplaceAll(value,"\t"," "), "\n"," ") }
		if err:=tabWriter.Write(row);err!=nil{return nil,err}
	}
	tabWriter.Flush()
	if err:=tabWriter.Error();err!=nil{return nil,err}
	return parseGroupCatalogueTXT(bytes.NewReader(tab.Bytes()))
}

func xmlText(value string) string {
	var out bytes.Buffer
	_ = xml.EscapeText(&out,[]byte(value))
	return out.String()
}

func xlsxCellRef(col,row int) string {
	col++
	name:=""
	for col>0 { rem:=(col-1)%26;name=string(rune('A'+rem))+name;col=(col-1)/26 }
	return name+strconv.Itoa(row)
}

func buildGroupCatalogueXLSX(rows [][]string) ([]byte,error) {
	var output bytes.Buffer
	archive:=zip.NewWriter(&output)
	entries:=map[string]string{
		"[Content_Types].xml":`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/><Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/></Types>`,
		"_rels/.rels":`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/></Relationships>`,
		"xl/workbook.xml":`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets><sheet name="Groups" sheetId="1" r:id="rId1"/></sheets></workbook>`,
		"xl/_rels/workbook.xml.rels":`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/></Relationships>`,
	}
	var sheet strings.Builder
	sheet.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?><worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData>`)
	for rowIndex,row:=range rows {
		sheet.WriteString("<row r=\"");sheet.WriteString(strconv.Itoa(rowIndex+1));sheet.WriteString("\">")
		for col,value:=range row {
			ref:=xlsxCellRef(col,rowIndex+1)
			sheet.WriteString("<c r=\"");sheet.WriteString(ref);sheet.WriteString("\" t=\"inlineStr\"><is><t xml:space=\"preserve\">")
			sheet.WriteString(xmlText(value));sheet.WriteString("</t></is></c>")
		}
		sheet.WriteString("</row>")
	}
	sheet.WriteString("</sheetData></worksheet>")
	entries["xl/worksheets/sheet1.xml"]=sheet.String()
	for name,body:=range entries {
		writer,e:=archive.Create(name);if e!=nil{archive.Close();return nil,e}
		if _,e=io.WriteString(writer,body);e!=nil{archive.Close();return nil,e}
	}
	if err:=archive.Close();err!=nil{return nil,err}
	return output.Bytes(),nil
}
