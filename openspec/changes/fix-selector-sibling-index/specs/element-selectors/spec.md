# Spec Delta

## Purpose

Defines how Omnisnap names an element in its outputs (causes, instances, intent relations, warnings and DOM audit findings), so that a selector a person pastes into the browser's devtools selects the element the report is about.

## ADDED Requirements

### Requirement: The capture records the DOM sibling position
For every captured element whose parent has two or more element children with the same tag, the capture SHALL record the element's 1-based position among those children. The count includes `display: none` elements and every other uncaptured element. It is omitted when the element is the only one of its tag.

#### Scenario: A hidden sibling is counted
- **GIVEN** a parent with three `div.card` children, the first of them `display: none`
- **WHEN** the page is captured
- **THEN** the two captured cards record positions 2 and 3

#### Scenario: A tag that is unique among its siblings has no position
- **GIVEN** a `main` that is the only `main` child of `body`
- **WHEN** the page is captured
- **THEN** the captured `main` node carries no sibling position

#### Scenario: The capture schema stays at version 1
- **WHEN** a capture with sibling positions is validated against `https://omnisnap.org/schema/capture/1.json`
- **THEN** it validates, and a capture written before this change also still validates

### Requirement: Positions in selectors follow the DOM
A selector segment SHALL carry `:nth-of-type(N)` exactly when the element's DOM parent has more than one element child with its tag, with N the element's DOM position. This holds even when only one of those children is rendered.

#### Scenario: The first visible card after a hidden one
- **GIVEN** a `div#projects-list` whose first `div.project-card` is `display: none`, and the second card overflows in one engine
- **WHEN** the cause is reported
- **THEN** its selector ends in `div.project-card:nth-of-type(2)`, not `:nth-of-type(1)`

#### Scenario: Only one of two same-tag siblings is rendered
- **GIVEN** a parent with two `p` children, the first `display: none`
- **WHEN** a cause is reported on the second `p`
- **THEN** its segment is `p:nth-of-type(2)`, so it does not select the hidden `p`

### Requirement: The same element has the same selector in every engine
The selector of an element SHALL NOT depend on which of its siblings a given engine renders.

#### Scenario: A sibling hidden in one engine only
- **GIVEN** a list whose second `li` is `display: none` in Firefox only, and a difference on the fourth `li`
- **WHEN** the page is compared with Chrome as the baseline
- **THEN** the cause names `li:nth-of-type(4)`, whichever side the cause is reported on

### Requirement: Selectors are valid CSS
Every selector Omnisnap writes SHALL parse as a CSS selector. Ids and class names are escaped as the CSSOM `CSS.escape()` function escapes them.

#### Scenario: A class with CSS-special characters
- **GIVEN** an element with the class `md:w-1/2`
- **WHEN** a cause is reported on it
- **THEN** its segment contains `.md\:w-1\/2`, and `document.querySelector` accepts the selector without throwing

#### Scenario: An id that starts with a digit
- **GIVEN** an element with the id `1col`
- **WHEN** a cause is reported on it
- **THEN** its segment is `div#\31 col`

### Requirement: An anchored selector selects the element
When a selector starts at an element with an id, at `body` or at `html`, `document.querySelector(selector)` in the captured page SHALL return the element the report is about. This assumes the anchor's id is unique in the page.

#### Scenario: Every cause on the planted suite
- **WHEN** the planted suite and the hidden-siblings fixture are compared in a real browser
- **THEN** for every cause, `document.querySelector` of its selector in the baseline page returns the element whose box the cause reports

### Requirement: A selector cut short still matches the element
A selector that reaches its five-segment limit before an id, `body` or `html` SHALL match the element, though it may also match other elements. `selector`'s description in the bundle schema says so.

#### Scenario: A deep element without an id nearby
- **GIVEN** an element nested more than five levels below its nearest ancestor with an id
- **WHEN** a cause is reported on it
- **THEN** the element is among the results of `document.querySelectorAll` of its selector

### Requirement: Captures without sibling positions keep their selectors
For a capture that records no sibling positions, such as one written by an earlier version or a golden fixture, selectors SHALL be computed exactly as before this change, apart from the escaping.

#### Scenario: Golden parity
- **WHEN** the golden parity tests run on the committed planted goldens, which were captured before this change
- **THEN** every cause has the same kinds, selector, side and deltas as the golden findings

### Requirement: DOM audit findings use the same selectors
A DOM audit finding SHALL name its element with the same selector a cause on that element would have.

#### Scenario: An audit finding on a repeated element
- **GIVEN** the third of three `button` siblings, which is zero-size and interactive
- **WHEN** the DOM audit reports it
- **THEN** the finding's selector ends in `button:nth-of-type(3)` and equals the selector a cause on that button would carry
