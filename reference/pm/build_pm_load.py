import json
from openpyxl import Workbook
from openpyxl.styles import Font, PatternFill, Alignment, Border, Side
from openpyxl.chart import LineChart, BarChart, Reference
from openpyxl.utils import get_column_letter as CL
from openpyxl.comments import Comment

d = json.load(open('agg.txt'))
PJ = json.load(open('pj.json'))
SPR = {"byesengeldi":45,"aiskakov":7,"drysmаndai":13,"ntazhibayev":100,"tkoyshybay":134,"vmashnin":170,"dbekturov":55,"damankeldy":3,"rserikov":1}
NAME = {"byesengeldi":"Есенгельды Бекнур","aiskakov":"Искаков Арсен","drysmаndai":"Рысмандай Диас","ntazhibayev":"Тажибаев Нурлыбек","tkoyshybay":"Қойшыбай Темірлан","vmashnin":"Машнин Владимир","dbekturov":"Бектуров Данияр","damankeldy":"Аманкелді Даниял","rserikov":"Серіков Рауан"}
TGT = ["byesengeldi","aiskakov","drysmаndai"]
others = sorted([l for l in NAME if l not in TGT], key=lambda l: -sum(a[0]+a[1] for a in d['PMP'][l].values()))
ORDER = TGT + others
WEEKS = ["2026-08-31","2026-09-07","2026-09-14","2026-09-21","2026-09-28","2026-10-05","2026-10-12","2026-10-19"]
WL = ["01.09–04.09","07.09–11.09","14.09–18.09","21.09–25.09","28.09–02.10","05.10–09.10","12.10–16.10","19.10–23.10"]

def w(l, i):
    return [d['W'][l].get(k, [0]*13)[i] for k in WEEKS]

def main_projects(l):
    res = []
    for k, a in sorted(d['PMP'][l].items(), key=lambda x: -(x[1][0]+x[1][1])):
        t = a[0]+a[1]
        if k in PJ and t >= 5 and t / PJ[k][1] >= 0.5:
            res.append(f"{k} ({round(100*t/PJ[k][1])}%)")
    return ", ".join(res) or "—"

F = "Arial"
fT = Font(name=F, size=14, bold=True)
fH = Font(name=F, size=10, bold=True, color="FFFFFF")
fB = Font(name=F, size=10)
fBold = Font(name=F, size=10, bold=True)
fIn = Font(name=F, size=10, color="0000FF")
fNote = Font(name=F, size=9, italic=True, color="555555")
fGreen = Font(name=F, size=10, color="008000")
HF = PatternFill("solid", fgColor="1F4E78")
TF = PatternFill("solid", fgColor="FFF2CC")
AF = PatternFill("solid", fgColor="E7E6E6")
thin = Side(style="thin", color="BFBFBF")
BR = Border(left=thin, right=thin, top=thin, bottom=thin)
WR = Alignment(wrap_text=True, vertical="center", horizontal="center")
LW = Alignment(wrap_text=True, vertical="center")

def hdr(ws, r, labels, c0=1):
    for i, t in enumerate(labels):
        c = ws.cell(r, c0+i, t); c.font = fH; c.fill = HF; c.alignment = WR; c.border = BR

def cell(ws, r, c, v, fmt=None, font=None, fill=None, al=None):
    x = ws.cell(r, c, v); x.font = font or fB; x.border = BR
    if fmt: x.number_format = fmt
    if fill: x.fill = fill
    x.alignment = al or Alignment(vertical="center", horizontal="center" if c > 1 else "left")
    return x

wb = Workbook()

# ---------------- Сводка ----------------
ws = wb.active; ws.title = "Сводка"
ws["A1"] = "Нагрузка ПМов: сентябрь vs октябрь 2026"; ws["A1"].font = fT
ws["A2"] = "Источник: Jira (jira.adata.kz), задачи, созданные с 01.09.2026; данные на 23.10.2026. Октябрь — неполный (1–23.10), поэтому сравнение идёт «в рабочий день». Жёлтым — Бекнур, Арсен, Диас."
ws["A2"].font = fNote
ws["A3"] = "Рабочих дней, сентябрь"; ws["A3"].font = fB
ws["C3"] = 22; ws["C3"].font = fIn
ws["D3"] = "Рабочих дней, 1–23 октября"; ws["D3"].font = fB
ws["F3"] = 17; ws["F3"].font = fIn
ws["C3"].comment = Comment("Будни 01–30.09.2026, праздников нет. Можно поправить вручную.", "Claude")
ws["F3"].comment = Comment("Будни 01–23.10.2026. Можно поправить вручную.", "Claude")

H = ["ПМ", "Основные проекты (доля его задач в проекте)", "Создано, сен", "Создано, окт (1–23)", "Создано в раб. день, сен", "Создано в раб. день, окт",
     "Δ темпа окт/сен", "Решено из его задач, сен", "Решено, окт", "Медиана срока закрытия, сен (дн.)", "Медиана, окт (дн.)",
     "Смен статусов самим ПМом (1.09–23.10)", "Смен исполнителя ПМом", "Возвратов в Need Rework по его задачам", "Возвратов на 100 задач",
     "Открыто задач (автор), всего", "из них в активном спринте", "из них созданы до 01.09", "Место по темпу постановки (окт)"]
hdr(ws, 5, H)
ws.row_dimensions[5].height = 66
r0 = 6
for i, l in enumerate(ORDER):
    r = r0 + i
    fill = TF if l in TGT else None
    sep = sum(a[0] for a in d['PMP'][l].values()); octv = sum(a[1] for a in d['PMP'][l].values())
    lt = d['LT'][l]
    cell(ws, r, 1, NAME[l], font=fBold if l in TGT else fB, fill=fill)
    cell(ws, r, 2, main_projects(l), fill=fill, al=LW)
    cell(ws, r, 3, sep, "#,##0", fIn, fill); cell(ws, r, 4, octv, "#,##0", fIn, fill)
    cell(ws, r, 5, f"=C{r}/$C$3", "0.0", fill=fill); cell(ws, r, 6, f"=D{r}/$F$3", "0.0", fill=fill)
    cell(ws, r, 7, f'=IFERROR(F{r}/E{r}-1,"")', '+0%;-0%;0%', fill=fill)
    cell(ws, r, 8, lt.get('09', [0])[0], "#,##0", fIn, fill); cell(ws, r, 9, lt.get('10', [0])[0], "#,##0", fIn, fill)
    cell(ws, r, 10, lt.get('09', [0, None])[1], "0.0", fIn, fill); cell(ws, r, 11, lt.get('10', [0, None])[1], "0.0", fIn, fill)
    cell(ws, r, 12, sum(w(l, 8)), "#,##0", fIn, fill); cell(ws, r, 13, sum(w(l, 10)), "#,##0", fIn, fill)
    cell(ws, r, 14, sum(w(l, 12)), "#,##0", fIn, fill)
    cell(ws, r, 15, f'=IFERROR(N{r}/(C{r}+D{r})*100,"")', "0.0", fill=fill)
    op = d['OP'][l]
    cell(ws, r, 16, op[0], "#,##0", fIn, fill); cell(ws, r, 17, SPR[l], "#,##0", fIn, fill); cell(ws, r, 18, op[2], "#,##0", fIn, fill)
    cell(ws, r, 19, f"=RANK(F{r},$F${r0}:$F${r0+len(ORDER)-1})", "0", fill=fill)
rl = r0 + len(ORDER) - 1
ra = rl + 1
cell(ws, ra, 1, "Среднее по остальным ПМ", font=fBold, fill=AF); cell(ws, ra, 2, "", fill=AF)
for c in range(3, 19):
    L = CL(c)
    cell(ws, ra, c, f"=AVERAGE({L}{r0+3}:{L}{rl})" if c != 7 else f'=IFERROR(F{ra}/E{ra}-1,"")', ws.cell(r0, c).number_format, fBold, AF)
cell(ws, ra, 19, "", fill=AF)
ws.cell(ra+2, 1, "Синий шрифт — числа из Jira, чёрный — формулы. Медиана срока = от создания до перехода в «Готово» для задач, созданных ПМом с 01.09 и закрытых в этом месяце.").font = fNote
ws.cell(ra+3, 1, "Смены статусов/исполнителя — действия самого ПМа в истории задач (двигает задачи по доске, переназначает) — показатель управленческой активности.").font = fNote
widths = [24, 34] + [11]*17
for i, x in enumerate(widths): ws.column_dimensions[CL(i+1)].width = x
for r in range(r0, ra+1): ws.row_dimensions[r].height = 30
ws.freeze_panes = "C6"

ch = BarChart(); ch.type = "bar"; ch.title = "Темп постановки задач: задач в рабочий день"; ch.style = 10
ch.add_data(Reference(ws, min_col=5, max_col=6, min_row=5, max_row=rl), titles_from_data=True)
ch.set_categories(Reference(ws, min_col=1, min_row=r0, max_row=rl))
ch.height = 10; ch.width = 22; ch.y_axis.title = "задач/день"; ch.x_axis.scaling.orientation = "maxMin"
ws.add_chart(ch, f"B{ra+5}")

# ---------------- Динамика ----------------
wd = wb.create_sheet("Динамика по неделям")
wd["A1"] = "Динамика по неделям (1.09–23.10.2026)"; wd["A1"].font = fT
wd["A2"] = "Первая неделя — 4 дня (1–4.09). Строка «Среднее по остальным» — среднее 6 других ПМов."; wd["A2"].font = fNote
blocks = [("Создано задач (ПМ — автор)", 0), ("Решено из задач ПМа (закрыто за неделю)", 7), ("Смен статусов самим ПМом", 8), ("Возвратов в Need Rework по задачам ПМа", 12)]
r = 4; starts = []
for title, idx in blocks:
    wd.cell(r, 1, title).font = Font(name=F, size=11, bold=True)
    hdr(wd, r+1, ["ПМ"] + WL + ["Итого", "Сен (нед. 1–4)", "Окт (нед. 6–8)"])
    rs = r + 2; starts.append(rs)
    for i, l in enumerate(ORDER):
        rr = rs + i; fill = TF if l in TGT else None
        cell(wd, rr, 1, NAME[l], font=fBold if l in TGT else fB, fill=fill)
        for j, v in enumerate(w(l, idx)): cell(wd, rr, 2+j, v, "#,##0", fIn, fill)
        cell(wd, rr, 10, f"=SUM(B{rr}:I{rr})", "#,##0", fill=fill)
        cell(wd, rr, 11, f"=SUM(B{rr}:E{rr})", "#,##0", fill=fill)
        cell(wd, rr, 12, f"=SUM(G{rr}:I{rr})", "#,##0", fill=fill)
    re_ = rs + len(ORDER) - 1; rav = re_ + 1
    cell(wd, rav, 1, "Среднее по остальным", font=fBold, fill=AF)
    for c in range(2, 13):
        L = CL(c); cell(wd, rav, c, f"=AVERAGE({L}{rs+3}:{L}{re_})", "0.0", fBold, AF)
    r = rav + 3
wd.cell(r-2, 1, "Неделя 28.09–02.10 переходная (сен/окт), поэтому в колонках «Сен» и «Окт» не учтена.").font = fNote
wd.column_dimensions["A"].width = 26
for c in range(2, 13): wd.column_dimensions[CL(c)].width = 12
wd.freeze_panes = "B4"
cr = r + 1
for bi, (title, idx) in enumerate(blocks[:3]):
    rs = starts[bi]; rav = rs + len(ORDER)
    lc = LineChart(); lc.title = title + ": Бекнур, Арсен, Диас vs среднее"; lc.style = 2; lc.height = 8; lc.width = 22; lc.legend.position = "b"
    for rr in [rs, rs+1, rs+2, rav]:
        lc.add_data(Reference(wd, min_col=1, max_col=9, min_row=rr, max_row=rr), from_rows=True, titles_from_data=True)
    for si, col in zip(lc.series, ["C00000", "2E75B6", "70AD47", "7F7F7F"]):
        si.smooth = False; si.graphicalProperties.line.solidFill = col; si.graphicalProperties.line.width = 28000
    lc.series[3].graphicalProperties.line.dashStyle = "dash"
    lc.set_categories(Reference(wd, min_col=2, max_col=9, min_row=rs-1, max_row=rs-1))
    wd.add_chart(lc, f"A{cr + bi*17}")

# ---------------- Качество ----------------
wq = wb.create_sheet("Качество постановки")
wq["A1"] = "Качество постановки задач (задачи, созданные ПМом 1.09–23.10)"; wq["A1"].font = fT
wq["A2"] = "Оценка времени (estimate) не заполняется ни у одного ПМа, поэтому не показана."; wq["A2"].font = fNote
hq = ["ПМ", "Создано", "С описанием ≥50 симв.", "В спринте", "С исполнителем", "Со сроком (due date)", "Баги", "% с описанием", "% в спринте", "% с исполнителем", "% со сроком", "% багов"]
hdr(wq, 4, hq); wq.row_dimensions[4].height = 40
for i, l in enumerate(ORDER):
    rr = 5 + i; fill = TF if l in TGT else None; sr = r0 + i
    cell(wq, rr, 1, NAME[l], font=fBold if l in TGT else fB, fill=fill)
    cell(wq, rr, 2, f"='Сводка'!C{sr}+'Сводка'!D{sr}", "#,##0", fGreen, fill)
    for j, idx in enumerate([2, 4, 5, 6, 1]): cell(wq, rr, 3+j, sum(w(l, idx)), "#,##0", fIn, fill)
    for j in range(5): cell(wq, rr, 8+j, f"=IFERROR({CL(3+j)}{rr}/$B{rr},0)", "0%", fill=fill)
rq = 5 + len(ORDER)
cell(wq, rq, 1, "Среднее по остальным", font=fBold, fill=AF)
for c in range(2, 13):
    L = CL(c); cell(wq, rq, c, f"=AVERAGE({L}8:{L}{rq-1})", "0%" if c >= 8 else "0", fBold, AF)
wq.column_dimensions["A"].width = 26
for c in range(2, 13): wq.column_dimensions[CL(c)].width = 13

# ---------------- Проекты ----------------
wp = wb.create_sheet("Проекты ПМов")
wp["A1"] = "Проекты ПМов: где ПМ — автор задач (1.09–23.10.2026)"; wp["A1"].font = fT
wp["A2"] = "«Основной» — ПМ автор ≥50% задач проекта за период. Исполнителей — разных исполнителей задач проекта за период."; wp["A2"].font = fNote
hp = ["ПМ", "Ключ", "Проект", "Создано ПМом, сен", "Создано ПМом, окт", "Всего ПМом", "Из них решено", "Из них открыто",
      "Всего задач в проекте", "Доля ПМа", "Решено в проекте", "Открыто в проекте", "Исполнителей в проекте", "Задач на исполнителя", "Основной проект"]
hdr(wp, 4, hp); wp.row_dimensions[4].height = 40
rr = 5
for l in ORDER:
    items = sorted(d['PMP'][l].items(), key=lambda x: -(x[1][0]+x[1][1]))
    for k, a in items:
        if a[0]+a[1] < 3 or k not in PJ: continue
        fill = TF if l in TGT else None; p = PJ[k]
        cell(wp, rr, 1, NAME[l], font=fBold if l in TGT else fB, fill=fill)
        cell(wp, rr, 2, k, fill=fill); cell(wp, rr, 3, p[0], fill=fill, al=LW)
        cell(wp, rr, 4, a[0], "#,##0", fIn, fill); cell(wp, rr, 5, a[1], "#,##0", fIn, fill)
        cell(wp, rr, 6, f"=D{rr}+E{rr}", "#,##0", fill=fill)
        cell(wp, rr, 7, a[2], "#,##0", fIn, fill); cell(wp, rr, 8, a[3], "#,##0", fIn, fill)
        cell(wp, rr, 9, p[1], "#,##0", fIn, fill); cell(wp, rr, 10, f"=IFERROR(F{rr}/I{rr},0)", "0%", fill=fill)
        cell(wp, rr, 11, p[2], "#,##0", fIn, fill); cell(wp, rr, 12, p[3], "#,##0", fIn, fill)
        cell(wp, rr, 13, p[4], "#,##0", fIn, fill); cell(wp, rr, 14, f"=IFERROR(I{rr}/M{rr},0)", "0.0", fill=fill)
        cell(wp, rr, 15, f'=IF(J{rr}>=0.5,"да","")', fill=fill)
        rr += 1
wp.auto_filter.ref = f"A4:O{rr-1}"
for i, x in enumerate([24, 10, 24] + [12]*12): wp.column_dimensions[CL(i+1)].width = x
wp.freeze_panes = "D5"

# ---------------- Методика ----------------
wm = wb.create_sheet("Методика")
notes = [
    "Как считалось",
    "Данные: Jira jira.adata.kz, выгрузка 23.10.2026 ~10:30 (Алматы). Период: 01.09–23.10.2026.",
    "ПМы для сравнения: все с ролью PM / основные постановщики задач (9 человек). Диас = Рысмандай Диас (SUP), Арсен = Искаков Арсен (DO), Бекнур = Есенгельды Бекнур (AA, LOGIST, TAI).",
    "Создано — задачи, где ПМ указан автором (reporter), по дате создания.",
    "Решено — задачи этого ПМа (созданные с 01.09), перешедшие в статус «Готово», по дате закрытия.",
    "Медиана срока закрытия — от создания до «Готово», в днях (календарных).",
    "Смены статусов / исполнителя — записи в истории задач, сделанные самим ПМом (по всем задачам, где он менял статус с 01.09).",
    "Need Rework — возвраты на доработку (переходы в статус Need Rework / Доработка) по задачам ПМа, созданным с 01.09.",
    "Открыто — незакрытые задачи ПМа как автора на 23.10 (за всё время); «в активном спринте» — в текущих открытых спринтах.",
    "Октябрь неполный (17 рабочих дней против 22 в сентябре), поэтому для сравнения используйте показатели «в рабочий день».",
    "Ограничения: комментарии и время в созвонах/переписке не учитываются; массовое создание задач (импорт, шаблоны) завышает «Создано» — см. всплески у Машнина (LOGIST, 5–16.10).",
]
for i, t in enumerate(notes):
    c = wm.cell(i+1, 1, t); c.font = fT if i == 0 else fB; c.alignment = Alignment(wrap_text=True, vertical="top")
wm.column_dimensions["A"].width = 120

wb.save("/mnt/user-data/outputs/Нагрузка_ПМов_сен-окт_2026.xlsx")
print("ok")
