#set page(paper: "a4", margin: 2.5cm, numbering: "1")
#set text(size: 11pt)
#set heading(numbering: "1.1")
#set par(justify: true)

#align(center)[
  #v(4cm)
  #text(size: 24pt, weight: "bold")[Project Report]
  #v(0.5cm)
  #text(size: 14pt)[A concise subtitle goes here]
  #v(1cm)
  Author Name \
  #datetime.today().display("[month repr:long] [day], [year]")
]

#pagebreak()
#outline()
#pagebreak()

= Introduction

State the problem, the context, and what this report covers. Typst makes it
easy to keep structure and styling separate — edit the rules at the top of
this file to restyle the whole document.

== Background

Cite prior work, describe constraints, and define terms.

= Method

Describe your approach. Formulas work inline like $E = m c^2$ or in display
mode:

$ integral_0^oo e^(-x^2) dif x = sqrt(pi) / 2 $

= Results

#figure(
  table(
    columns: 3,
    table.header([*Metric*], [*Baseline*], [*Ours*]),
    [Accuracy], [82.1%], [91.4%],
    [Latency], [120 ms], [45 ms],
  ),
  caption: [Headline results.],
)

= Conclusion

Summarize findings and future work.
