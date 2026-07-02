#set page(paper: "a4", margin: (x: 2.5cm, y: 2.5cm))
#set text(size: 11pt)

Jane Doe \
Example Street 12 \
12345 Sampletown

#v(1cm)
#align(right)[
  Acme Corp \
  Attn: Hiring Team \
  Business Road 1 \
  54321 Workville
]

#v(1cm)
#align(right)[#datetime.today().display("[month repr:long] [day], [year]")]

#v(0.5cm)
*Subject: Your subject line here*

#v(0.5cm)
Dear Sir or Madam,

Write the body of your letter here. Keep paragraphs short and to the point.
A second paragraph can expand on details, provide context, or list the
specific items you are enclosing or requesting.

Thank you for your time and consideration.

#v(1.2cm)
Kind regards,

#v(1.5cm)
Jane Doe
