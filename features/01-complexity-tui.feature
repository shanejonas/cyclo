# Implementation: github.com/shanejonas/cyclomatic-complexity-tui/adapters/inbound/tui
Feature: Explore Go complexity in the terminal
  Developers can find risky Go code without leaving a compact terminal workflow.

  Scenario: Start with the current repository
    Given the complexity TUI is started without a path
    When its own repository has been analyzed
    Then the current directory is the scan target
    And its production functions have complexity at most 10
    And the scan summary is visible

  Scenario: Rank files and functions by complexity
    Given an analyzed path contains Go files with different complexity
    When the wide complexity view is shown
    Then files are ranked by aggregate complexity
    And the selected file's functions are ranked by complexity
    And function details show cyclomatic paths, cognitive load, and physical size
    And cognitive increments are purple in the source
    And a compact header, focus gutter, and muted green and blue palette frame the view

  Scenario: Navigate files and functions
    Given the wide complexity view has multiple files and functions
    When the developer moves between panes and ranked entries
    Then the focus gutter follows the selected entry
    And the detail pane follows the selected file and function

  Scenario: Refresh an analysis
    Given the analyzed path has changed since the view loaded
    When the developer presses "r"
    Then the same path is analyzed again
    And the refreshed ranking replaces the stale results

  Scenario: Keep narrow terminals useful and easy to leave
    Given the terminal is too narrow for three panes
    When the developer cycles pane focus
    Then one complete focused pane is visible at a time
    And no rendered line exceeds the terminal width
    And the compact key footer remains visible
    And either "q" or "ctrl+c" quits
