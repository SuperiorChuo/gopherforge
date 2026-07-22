# Excel 导入导出

管理后台的肌肉记忆能力：列表一键导出、模板化批量导入，由共享组件 `shared/pkg/excel` 提供（excelize 流式写出，内存有界）。

## 用户导出

用户列表页「导出」按当前筛选条件（关键字/状态）导出 xlsx，**复用列表权限与数据范围**——你能看到什么就只能导出什么。分页循环拉取，单次上限 1 万行（超限明确截断提示，不静默）。

## 用户批量导入

1. 「导入 → 下载导入模板」得到带示例行的 xlsx。
2. 填写：用户名（必填）、昵称、初始密码（留空用默认）、邮箱、手机号、部门名称（须已存在）、状态。
3. 上传后**部分成功语义**：单行失败不中断其余行，逐行错误明细（行号/用户名/原因）返回展示——重名、部门不存在、配额超限等一目了然。

落库复用创建用户的全部校验（租户配额、重名、越权部门），导入不开后门。

## 在你的业务里复用

```go
import "github.com/go-admin-kit/services/shared/pkg/excel"

sheet, _ := excel.NewSheet("订单", []string{"ID", "客户", "金额"}, []float64{8, 24, 12})
for _, o := range orders {
    _ = sheet.AppendRow(o.ID, o.Customer, float64(o.AmountCents)/100)
}
_ = sheet.WriteHTTP(c.Writer, "orders.xlsx")   // 自动设下载头
```

导入解析用 `excel.ReadFirstSheet(reader, maxRows)`（超行数上限报错而非截断）。前端配套通用组件 `ExcelImportModal`（模板下载/拖拽上传/逐行错误表）。
