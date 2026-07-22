# Excel Import/Export

Provided by the shared `shared/pkg/excel` component (excelize stream writer, bounded memory).

- **User export**: current filters, respecting list permissions and data scopes; paged fetch with an explicit 10k-row cap.
- **User import**: download a template, fill rows, upload. **Partial-success semantics** — a failing row doesn't abort the rest; per-row errors (row/username/reason) are returned. All create-user validations (quota, duplicates, department bounds) apply.

Reuse in your service:

```go
sheet, _ := excel.NewSheet("Orders", []string{"ID", "Customer", "Amount"}, nil)
for _, o := range orders { _ = sheet.AppendRow(o.ID, o.Customer, float64(o.AmountCents)/100) }
_ = sheet.WriteHTTP(c.Writer, "orders.xlsx")
```

Parsing uses `excel.ReadFirstSheet(reader, maxRows)` (errors on overflow instead of silent truncation). The frontend ships a generic `ExcelImportModal` (template download / drag-drop / per-row error table).
