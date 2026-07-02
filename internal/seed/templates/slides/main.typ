#import "@preview/polylux:0.4.0": *

#set page(paper: "presentation-16-9")
#set text(size: 22pt)

#slide[
  #align(horizon + center)[
    #text(size: 36pt, weight: "bold")[Presentation Title]

    #text(size: 20pt)[Author Name · #datetime.today().display("[year]")]
  ]
]

#slide[
  == Agenda

  + The problem
  + Our approach
  + Results
  + Next steps
]

#slide[
  == The problem

  - State the pain point clearly
  - One idea per bullet
  - Keep slides sparse — speak the rest
]

#slide[
  == Results

  #align(center)[
    #table(
      columns: 2,
      table.header([*Metric*], [*Value*]),
      [Throughput], [2.4×],
      [Cost], [−38%],
    )
  ]
]

#slide[
  #align(horizon + center)[
    #text(size: 32pt, weight: "bold")[Thank you!]

    Questions?
  ]
]
