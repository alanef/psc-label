// Program to produce labels for PSC in 60mm x 40mm layout
package main

import (
	"encoding/csv"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"sort"
	"strings"
	"strconv"
	"time"

	"github.com/jung-kurt/gofpdf"
	"github.com/skratchdot/open-golang/open"
)

// the UI
//
const testpx = `Hello World`
const testp = `
<!DOCTYPE html>
<html lang="en">
<head>

  <!-- Basic Page Needs
  –––––––––––––––––––––––––––––––––––––––––––––––––– -->
  <meta charset="utf-8">
  <title>Label printing</title>
  <meta name="description" content="">
  <meta name="author" content="">

  <!-- Mobile Specific Metas
  –––––––––––––––––––––––––––––––––––––––––––––––––– -->
  <meta name="viewport" content="width=device-width, initial-scale=1">

  <!-- FONT
  –––––––––––––––––––––––––––––––––––––––––––––––––– -->
  <link href="//fonts.googleapis.com/css?family=Raleway:400,300,600" rel="stylesheet" type="text/css">

  <!-- CSS
  –––––––––––––––––––––––––––––––––––––––––––––––––– -->
	<style>
	/*! normalize.css v3.0.2 | MIT License | git.io/normalize */

	/**
	 * 1. Set default font family to sans-serif.
	 * 2. Prevent iOS text size adjust after orientation change, without disabling
	 *    user zoom.
	 */

	html {
	  font-family: sans-serif; /* 1 */
	  -ms-text-size-adjust: 100%; /* 2 */
	  -webkit-text-size-adjust: 100%; /* 2 */
	}

	/**
	 * Remove default margin.
	 */

	body {
	  margin: 0;
	}

	/* HTML5 display definitions
	   ========================================================================== */

	/**
	 * Correct 'block' display not defined for any HTML5 element in IE 8/9.
	 * Correct 'block' display not defined for 'details' or 'summary' in IE 10/11
	 * and Firefox.
	 * Correct 'block' display not defined for 'main' in IE 11.
	 */

	article,
	aside,
	details,
	figcaption,
	figure,
	footer,
	header,
	hgroup,
	main,
	menu,
	nav,
	section,
	summary {
	  display: block;
	}

	/**
	 * 1. Correct 'inline-block' display not defined in IE 8/9.
	 * 2. Normalize vertical alignment of 'progress' in Chrome, Firefox, and Opera.
	 */

	audio,
	canvas,
	progress,
	video {
	  display: inline-block; /* 1 */
	  vertical-align: baseline; /* 2 */
	}

	/**
	 * Prevent modern browsers from displaying 'audio' without controls.
	 * Remove excess height in iOS 5 devices.
	 */

	audio:not([controls]) {
	  display: none;
	  height: 0;
	}

	/**
	 * Address '[hidden]' styling not present in IE 8/9/10.
	 * Hide the 'template' element in IE 8/9/11, Safari, and Firefox < 22.
	 */

	[hidden],
	template {
	  display: none;
	}

	/* Links
	   ========================================================================== */

	/**
	 * Remove the gray background color from active links in IE 10.
	 */

	a {
	  background-color: transparent;
	}

	/**
	 * Improve readability when focused and also mouse hovered in all browsers.
	 */

	a:active,
	a:hover {
	  outline: 0;
	}

	/* Text-level semantics
	   ========================================================================== */

	/**
	 * Address styling not present in IE 8/9/10/11, Safari, and Chrome.
	 */

	abbr[title] {
	  border-bottom: 1px dotted;
	}

	/**
	 * Address style set to 'bolder' in Firefox 4+, Safari, and Chrome.
	 */

	b,
	strong {
	  font-weight: bold;
	}

	/**
	 * Address styling not present in Safari and Chrome.
	 */

	dfn {
	  font-style: italic;
	}

	/**
	 * Address variable 'h1' font-size and margin within 'section' and 'article'
	 * contexts in Firefox 4+, Safari, and Chrome.
	 */

	h1 {
	  font-size: 2em;
	  margin: 0.67em 0;
	}

	/**
	 * Address styling not present in IE 8/9.
	 */

	mark {
	  background: #ff0;
	  color: #000;
	}

	/**
	 * Address inconsistent and variable font size in all browsers.
	 */

	small {
	  font-size: 80%;
	}

	/**
	 * Prevent 'sub' and 'sup' affecting 'line-height' in all browsers.
	 */

	sub,
	sup {
	  font-size: 75%;
	  line-height: 0;
	  position: relative;
	  vertical-align: baseline;
	}

	sup {
	  top: -0.5em;
	}

	sub {
	  bottom: -0.25em;
	}

	/* Embedded content
	   ========================================================================== */

	/**
	 * Remove border when inside 'a' element in IE 8/9/10.
	 */

	img {
	  border: 0;
	}

	/**
	 * Correct overflow not hidden in IE 9/10/11.
	 */

	svg:not(:root) {
	  overflow: hidden;
	}

	/* Grouping content
	   ========================================================================== */

	/**
	 * Address margin not present in IE 8/9 and Safari.
	 */

	figure {
	  margin: 1em 40px;
	}

	/**
	 * Address differences between Firefox and other browsers.
	 */

	hr {
	  -moz-box-sizing: content-box;
	  box-sizing: content-box;
	  height: 0;
	}

	/**
	 * Contain overflow in all browsers.
	 */

	pre {
	  overflow: auto;
	}

	/**
	 * Address odd 'em'-unit font size rendering in all browsers.
	 */

	code,
	kbd,
	pre,
	samp {
	  font-family: monospace, monospace;
	  font-size: 1em;
	}

	/* Forms
	   ========================================================================== */

	/**
	 * Known limitation: by default, Chrome and Safari on OS X allow very limited
	 * styling of 'select', unless a 'border' property is set.
	 */

	/**
	 * 1. Correct color not being inherited.
	 *    Known issue: affects color of disabled elements.
	 * 2. Correct font properties not being inherited.
	 * 3. Address margins set differently in Firefox 4+, Safari, and Chrome.
	 */

	button,
	input,
	optgroup,
	select,
	textarea {
	  color: inherit; /* 1 */
	  font: inherit; /* 2 */
	  margin: 0; /* 3 */
	}

	/**
	 * Address 'overflow' set to 'hidden' in IE 8/9/10/11.
	 */

	button {
	  overflow: visible;
	}

	/**
	 * Address inconsistent 'text-transform' inheritance for 'button' and 'select'.
	 * All other form control elements do not inherit 'text-transform' values.
	 * Correct 'button' style inheritance in Firefox, IE 8/9/10/11, and Opera.
	 * Correct 'select' style inheritance in Firefox.
	 */

	button,
	select {
	  text-transform: none;
	}

	/**
	 * 1. Avoid the WebKit bug in Android 4.0.* where (2) destroys native 'audio'
	 *    and 'video' controls.
	 * 2. Correct inability to style clickable 'input' types in iOS.
	 * 3. Improve usability and consistency of cursor style between image-type
	 *    'input' and others.
	 */

	button,
	html input[type="button"], /* 1 */
	input[type="reset"],
	input[type="submit"] {
	  -webkit-appearance: button; /* 2 */
	  cursor: pointer; /* 3 */
	}

	/**
	 * Re-set default cursor for disabled elements.
	 */

	button[disabled],
	html input[disabled] {
	  cursor: default;
	}

	/**
	 * Remove inner padding and border in Firefox 4+.
	 */

	button::-moz-focus-inner,
	input::-moz-focus-inner {
	  border: 0;
	  padding: 0;
	}

	/**
	 * Address Firefox 4+ setting 'line-height' on 'input' using '!important' in
	 * the UA stylesheet.
	 */

	input {
	  line-height: normal;
	}

	/**
	 * It's recommended that you don't attempt to style these elements.
	 * Firefox's implementation doesn't respect box-sizing, padding, or width.
	 *
	 * 1. Address box sizing set to 'content-box' in IE 8/9/10.
	 * 2. Remove excess padding in IE 8/9/10.
	 */

	input[type="checkbox"],
	input[type="radio"] {
	  box-sizing: border-box; /* 1 */
	  padding: 0; /* 2 */
	}

	/**
	 * Fix the cursor style for Chrome's increment/decrement buttons. For certain
	 * 'font-size' values of the 'input', it causes the cursor style of the
	 * decrement button to change from 'default' to 'text'.
	 */

	input[type="number"]::-webkit-inner-spin-button,
	input[type="number"]::-webkit-outer-spin-button {
	  height: auto;
	}

	/**
	 * 1. Address 'appearance' set to 'searchfield' in Safari and Chrome.
	 * 2. Address 'box-sizing' set to 'border-box' in Safari and Chrome
	 *    (include '-moz' to future-proof).
	 */

	input[type="search"] {
	  -webkit-appearance: textfield; /* 1 */
	  -moz-box-sizing: content-box;
	  -webkit-box-sizing: content-box; /* 2 */
	  box-sizing: content-box;
	}

	/**
	 * Remove inner padding and search cancel button in Safari and Chrome on OS X.
	 * Safari (but not Chrome) clips the cancel button when the search input has
	 * padding (and 'textfield' appearance).
	 */

	input[type="search"]::-webkit-search-cancel-button,
	input[type="search"]::-webkit-search-decoration {
	  -webkit-appearance: none;
	}

	/**
	 * Define consistent border, margin, and padding.
	 */

	fieldset {
	  border: 1px solid #c0c0c0;
	  margin: 0 2px;
	  padding: 0.35em 0.625em 0.75em;
	}

	/**
	 * 1. Correct 'color' not being inherited in IE 8/9/10/11.
	 * 2. Remove padding so people aren't caught out if they zero out fieldsets.
	 */

	legend {
	  border: 0; /* 1 */
	  padding: 0; /* 2 */
	}

	/**
	 * Remove default vertical scrollbar in IE 8/9/10/11.
	 */

	textarea {
	  overflow: auto;
	}

	/**
	 * Don't inherit the 'font-weight' (applied by a rule above).
	 * NOTE: the default cannot safely be changed in Chrome and Safari on OS X.
	 */

	optgroup {
	  font-weight: bold;
	}

	/* Tables
	   ========================================================================== */

	/**
	 * Remove most spacing between table cells.
	 */

	table {
	  border-collapse: collapse;
	  border-spacing: 0;
	}

	td,
	th {
	  padding: 0;
	}

	/* ------------------------ end normailize ------------------- */

	/*
	* Skeleton V2.0.4
	* Copyright 2014, Dave Gamache
	* www.getskeleton.com
	* Free to use under the MIT license.
	* http://www.opensource.org/licenses/mit-license.php
	* 12/29/2014
	*/


	/* Table of contents
	––––––––––––––––––––––––––––––––––––––––––––––––––
	- Grid
	- Base Styles
	- Typography
	- Links
	- Buttons
	- Forms
	- Lists
	- Code
	- Tables
	- Spacing
	- Utilities
	- Clearing
	- Media Queries
	*/


	/* Grid
	–––––––––––––––––––––––––––––––––––––––––––––––––– */
	.container {
	  position: relative;
	  width: 100%;
	  max-width: 960px;
	  margin: 0 auto;
	  padding: 0 20px;
	  box-sizing: border-box; }
	.column,
	.columns {
	  width: 100%;
	  float: left;
	  box-sizing: border-box; }

	/* For devices larger than 400px */
	@media (min-width: 400px) {
	  .container {
	    width: 85%;
	    padding: 0; }
	}

	/* For devices larger than 550px */
	@media (min-width: 550px) {
	  .container {
	    width: 80%; }
	  .column,
	  .columns {
	    margin-left: 4%; }
	  .column:first-child,
	  .columns:first-child {
	    margin-left: 0; }

	  .one.column,
	  .one.columns                    { width: 4.66666666667%; }
	  .two.columns                    { width: 13.3333333333%; }
	  .three.columns                  { width: 22%;            }
	  .four.columns                   { width: 30.6666666667%; }
	  .five.columns                   { width: 39.3333333333%; }
	  .six.columns                    { width: 48%;            }
	  .seven.columns                  { width: 56.6666666667%; }
	  .eight.columns                  { width: 65.3333333333%; }
	  .nine.columns                   { width: 74.0%;          }
	  .ten.columns                    { width: 82.6666666667%; }
	  .eleven.columns                 { width: 91.3333333333%; }
	  .twelve.columns                 { width: 100%; margin-left: 0; }

	  .one-third.column               { width: 30.6666666667%; }
	  .two-thirds.column              { width: 65.3333333333%; }

	  .one-half.column                { width: 48%; }

	  /* Offsets */
	  .offset-by-one.column,
	  .offset-by-one.columns          { margin-left: 8.66666666667%; }
	  .offset-by-two.column,
	  .offset-by-two.columns          { margin-left: 17.3333333333%; }
	  .offset-by-three.column,
	  .offset-by-three.columns        { margin-left: 26%;            }
	  .offset-by-four.column,
	  .offset-by-four.columns         { margin-left: 34.6666666667%; }
	  .offset-by-five.column,
	  .offset-by-five.columns         { margin-left: 43.3333333333%; }
	  .offset-by-six.column,
	  .offset-by-six.columns          { margin-left: 52%;            }
	  .offset-by-seven.column,
	  .offset-by-seven.columns        { margin-left: 60.6666666667%; }
	  .offset-by-eight.column,
	  .offset-by-eight.columns        { margin-left: 69.3333333333%; }
	  .offset-by-nine.column,
	  .offset-by-nine.columns         { margin-left: 78.0%;          }
	  .offset-by-ten.column,
	  .offset-by-ten.columns          { margin-left: 86.6666666667%; }
	  .offset-by-eleven.column,
	  .offset-by-eleven.columns       { margin-left: 95.3333333333%; }

	  .offset-by-one-third.column,
	  .offset-by-one-third.columns    { margin-left: 34.6666666667%; }
	  .offset-by-two-thirds.column,
	  .offset-by-two-thirds.columns   { margin-left: 69.3333333333%; }

	  .offset-by-one-half.column,
	  .offset-by-one-half.columns     { margin-left: 52%; }

	}


	/* Base Styles
	–––––––––––––––––––––––––––––––––––––––––––––––––– */
	/* NOTE
	html is set to 62.5% so that all the REM measurements throughout Skeleton
	are based on 10px sizing. So basically 1.5rem = 15px :) */
	html {
	  font-size: 62.5%; }
	body {
	  font-size: 1.5em; /* currently ems cause chrome bug misinterpreting rems on body element */
	  line-height: 1.6;
	  font-weight: 400;
	  font-family: "Raleway", "HelveticaNeue", "Helvetica Neue", Helvetica, Arial, sans-serif;
	  color: #222; }


	/* Typography
	–––––––––––––––––––––––––––––––––––––––––––––––––– */
	h1, h2, h3, h4, h5, h6 {
	  margin-top: 0;
	  margin-bottom: 2rem;
	  font-weight: 300; }
	h1 { font-size: 4.0rem; line-height: 1.2;  letter-spacing: -.1rem;}
	h2 { font-size: 3.6rem; line-height: 1.25; letter-spacing: -.1rem; }
	h3 { font-size: 3.0rem; line-height: 1.3;  letter-spacing: -.1rem; }
	h4 { font-size: 2.4rem; line-height: 1.35; letter-spacing: -.08rem; }
	h5 { font-size: 1.8rem; line-height: 1.5;  letter-spacing: -.05rem; }
	h6 { font-size: 1.5rem; line-height: 1.6;  letter-spacing: 0; }

	/* Larger than phablet */
	@media (min-width: 550px) {
	  h1 { font-size: 5.0rem; }
	  h2 { font-size: 4.2rem; }
	  h3 { font-size: 3.6rem; }
	  h4 { font-size: 3.0rem; }
	  h5 { font-size: 2.4rem; }
	  h6 { font-size: 1.5rem; }
	}

	p {
	  margin-top: 0; }


	/* Links
	–––––––––––––––––––––––––––––––––––––––––––––––––– */
	a {
	  color: #1EAEDB; }
	a:hover {
	  color: #0FA0CE; }


	/* Buttons
	–––––––––––––––––––––––––––––––––––––––––––––––––– */
	.button,
	button,
	input[type="submit"],
	input[type="reset"],
	input[type="button"] {
	  display: inline-block;
	  height: 38px;
	  padding: 0 30px;
	  color: #555;
	  text-align: center;
	  font-size: 11px;
	  font-weight: 600;
	  line-height: 38px;
	  letter-spacing: .1rem;
	  text-transform: uppercase;
	  text-decoration: none;
	  white-space: nowrap;
	  background-color: transparent;
	  border-radius: 4px;
	  border: 1px solid #bbb;
	  cursor: pointer;
	  box-sizing: border-box; }
	.button:hover,
	button:hover,
	input[type="submit"]:hover,
	input[type="reset"]:hover,
	input[type="button"]:hover,
	.button:focus,
	button:focus,
	input[type="submit"]:focus,
	input[type="reset"]:focus,
	input[type="button"]:focus {
	  color: #333;
	  border-color: #888;
	  outline: 0; }
	.button.button-primary,
	button.button-primary,
	input[type="submit"].button-primary,
	input[type="reset"].button-primary,
	input[type="button"].button-primary {
	  color: #FFF;
	  background-color: #33C3F0;
	  border-color: #33C3F0; }
	.button.button-primary:hover,
	button.button-primary:hover,
	input[type="submit"].button-primary:hover,
	input[type="reset"].button-primary:hover,
	input[type="button"].button-primary:hover,
	.button.button-primary:focus,
	button.button-primary:focus,
	input[type="submit"].button-primary:focus,
	input[type="reset"].button-primary:focus,
	input[type="button"].button-primary:focus {
	  color: #FFF;
	  background-color: #1EAEDB;
	  border-color: #1EAEDB; }


	/* Forms
	–––––––––––––––––––––––––––––––––––––––––––––––––– */
	input[type="email"],
	input[type="number"],
	input[type="search"],
	input[type="text"],
	input[type="tel"],
	input[type="url"],
	input[type="password"],
	textarea,
	select {
	  height: 38px;
	  padding: 6px 10px; /* The 6px vertically centers text on FF, ignored by Webkit */
	  background-color: #fff;
	  border: 1px solid #D1D1D1;
	  border-radius: 4px;
	  box-shadow: none;
	  box-sizing: border-box; }
	/* Removes awkward default styles on some inputs for iOS */
	input[type="email"],
	input[type="number"],
	input[type="search"],
	input[type="text"],
	input[type="tel"],
	input[type="url"],
	input[type="password"],
	textarea {
	  -webkit-appearance: none;
	     -moz-appearance: none;
	          appearance: none; }
	textarea {
	  min-height: 65px;
	  padding-top: 6px;
	  padding-bottom: 6px; }
	input[type="email"]:focus,
	input[type="number"]:focus,
	input[type="search"]:focus,
	input[type="text"]:focus,
	input[type="tel"]:focus,
	input[type="url"]:focus,
	input[type="password"]:focus,
	textarea:focus,
	select:focus {
	  border: 1px solid #33C3F0;
	  outline: 0; }
	label,
	legend {
	  display: block;
	  margin-bottom: .5rem;
	  font-weight: 600; }
	fieldset {
	  padding: 0;
	  border-width: 0; }
	input[type="checkbox"],
	input[type="radio"] {
	  display: inline; }
	label > .label-body {
	  display: inline-block;
	  margin-left: .5rem;
	  font-weight: normal; }


	/* Lists
	–––––––––––––––––––––––––––––––––––––––––––––––––– */
	ul {
	  list-style: circle inside; }
	ol {
	  list-style: decimal inside; }
	ol, ul {
	  padding-left: 0;
	  margin-top: 0; }
	ul ul,
	ul ol,
	ol ol,
	ol ul {
	  margin: 1.5rem 0 1.5rem 3rem;
	  font-size: 90%; }
	li {
	  margin-bottom: 1rem; }


	/* Code
	–––––––––––––––––––––––––––––––––––––––––––––––––– */
	code {
	  padding: .2rem .5rem;
	  margin: 0 .2rem;
	  font-size: 90%;
	  white-space: nowrap;
	  background: #F1F1F1;
	  border: 1px solid #E1E1E1;
	  border-radius: 4px; }
	pre > code {
	  display: block;
	  padding: 1rem 1.5rem;
	  white-space: pre; }


	/* Tables
	–––––––––––––––––––––––––––––––––––––––––––––––––– */
	th,
	td {
	  padding: 12px 15px;
	  text-align: left;
	  border-bottom: 1px solid #E1E1E1; }
	th:first-child,
	td:first-child {
	  padding-left: 0; }
	th:last-child,
	td:last-child {
	  padding-right: 0; }


	/* Spacing
	–––––––––––––––––––––––––––––––––––––––––––––––––– */
	button,
	.button {
	  margin-bottom: 1rem; }
	input,
	textarea,
	select,
	fieldset {
	  margin-bottom: 1.5rem; }
	pre,
	blockquote,
	dl,
	figure,
	table,
	p,
	ul,
	ol,
	form {
	  margin-bottom: 2.5rem; }


	/* Utilities
	–––––––––––––––––––––––––––––––––––––––––––––––––– */
	.u-full-width {
	  width: 100%;
	  box-sizing: border-box; }
	.u-max-full-width {
	  max-width: 100%;
	  box-sizing: border-box; }
	.u-pull-right {
	  float: right; }
	.u-pull-left {
	  float: left; }


	/* Misc
	–––––––––––––––––––––––––––––––––––––––––––––––––– */
	hr {
	  margin-top: 3rem;
	  margin-bottom: 3.5rem;
	  border-width: 0;
	  border-top: 1px solid #E1E1E1; }


	/* Clearing
	–––––––––––––––––––––––––––––––––––––––––––––––––– */

	/* Self Clearing Goodness */
	.container:after,
	.row:after,
	.u-cf {
	  content: "";
	  display: table;
	  clear: both; }


	/* Media Queries
	–––––––––––––––––––––––––––––––––––––––––––––––––– */
	/*
	Note: The best way to structure the use of media queries is to create the queries
	near the relevant code. For example, if you wanted to change the styles for buttons
	on small devices, paste the mobile query code up in the buttons section and style it
	there.
	*/


	/* Larger than mobile */
	@media (min-width: 400px) {}

	/* Larger than phablet (also point when grid becomes active) */
	@media (min-width: 550px) {}

	/* Larger than tablet */
	@media (min-width: 750px) {}

	/* Larger than desktop */
	@media (min-width: 1000px) {}

	/* Larger than Desktop HD */
	@media (min-width: 1200px) {}


/* ---------------- end skeleton --------------- */

/* Table of contents for custom
–––––––––––––––––––––––––––––––––––––––––––––––––– */
.error {
  background-color: red;
  padding: 10px;
  color: #fff;
  font-size: 150%;
}



  <!-- Favicon
  –––––––––––––––––––––––––––––––––––––––––––––––––– -->
  <!-- <link rel="icon" type="image/png" href="static/images/favicon.png"> -->

</style>

</head>
<body>
<!-- Primary Page Layout
  –––––––––––––––––––––––––––––––––––––––––––––––––– -->

<div class="container">
  <h1>PSC Berth Label Printing {{ .Year }}</h1>
  <div class="row">
    <div class="six columns">
      <div class="row">
        <h2>Individual Label</h2>
      </div>
      <div class="row">
          <form action="/printsingle" method="post">
            <div class="row">
              <div class="six columns">
                <label for="Name">Member's Name</label>
                <input class="u-full-width" type="text" placeholder="Member Full Name" name="Name">
              </div>
              <div class="six columns">
                <label for="Boat">Boat Class and Sail No</label>
                <input class="u-full-width" type="text" placeholder="e.g. Solo:1246" name="Boat">
              </div>
            </div>
            <div class="row">
              <div class="six columns">
                <label for="Boat">Berth</label>
                <input class="u-full-width" type="text" placeholder="e.g. C147" name="Berth">
             </div>
             <div class="six columns">
               <label for="submit">&nbsp;</label>
               <input class="button-primary" type="submit" value="Create">
             </div>
           </div>
          </form>
          {{ with .Singleerr}}
          <div class="row">
            <p class="error">
              {{ range .}}{{.}}{{end}}
            </p>
          </div>
          {{ end }}

       </div>
    </div>

    <div class="six columns">
      {{ with .Labelfile }}
        <iframe id="iFramePdf" src="{{ . }}" style="height: 250px"></iframe>
        <div class="row">
          <input  class="button-primary" type="button" value="Print" onclick="printTrigger('iFramePdf');" />
					<div class="row">
						 <p>Make sure the printer is set up correctly, the labels are designed for 60mm wide x 40mm high print area, with 1.3 ( one point three)mm non print margin.
						 You may need to go Printer > Preferences and create a new USER defined page size. Make sure you select the correct size when you print and select 'fit to page'.
						 If you are printing bulk print a test range first.</p>

					</div>
        </div>
        <script>
        function printTrigger(elementId) {
            var getMyFrame = document.getElementById(elementId);
            getMyFrame.focus();
            getMyFrame.contentWindow.print();
        }
        </script>
     {{ end }}

  </div>
  <div class="row">
      <div class="twelve columns">
      <hr>
      <div class="row">
        <h2>Bulk Labels</h2>
      </div>
      <div class="row">
			     <p>Bulk labels, first download the Mooring allocation and Boat files from SCM, upload them and process. The labels are driven from the Mooring Allocations CSV
				 so if you want you can remove uneeded rows from the Mooring CSV before printing, or even sort the file in spreadsheet and re save as as CSV. But do not delete the header it must be the first row, and do not delete any columns.
				   </p>
          <form action="/printbulk" method="post" enctype="multipart/form-data">
            <div class="row">
              <div class="four columns">
							  <div class="row">
								  <a href="https://papercourtsc.clubmin.net/moorings/export_setup" target="_blank" class="button">Download SCM Moorings</a>
								</div>
								<div class="row">
	                <label for="Name">Mooring Allocations CSV</label>
	                <input class="u-full-width" type="file" name="BerthFile">
								</div>
              </div>
              <div class="four columns">
								<div class="row">
									<a href="https://papercourtsc.clubmin.net/boats/export" target="_blank" class="button">Download SCM Boats</a>
								</div>
								<div class="row">
	                <label for="Boat">Boat CSV</label>
	                <input class="u-full-width" type="file"  name="BoatFile">
	              </div>
							</div>
              <div class="four columns">
               <label for="submit">&nbsp;</label>
               <input class="button-primary" type="submit" value="Process">
             </div>
           </div>
          </form>
          {{ with .Bulkerr}}
          <div class="row">
            <p class="error">
              {{ range .}}{{.}}{{end}}
            </p>
          </div>
          {{ end }}
      </div>
   </div>
 </div>
 <div class="row" id="number">
      <div class="twelve columns">
      <hr>
      <div class="row">
        <h2>Number Labels</h2>
      </div>
      <div class="row">
		  <form action="/printnumber" method="post">
	
        <div class="row">
              <div class="six columns">
                <label for="From">From Number</label>
                <input class="u-full-width" type="number"  min="1" max="9999" name="From">
              </div>
              <div class="six columns">
                <label for="To">To Number</label>
                <input class="u-full-width" type="number"  min="1" max="9999" name="To">
              </div>
            </div>
             <div class="six columns">
               <label for="submit">&nbsp;</label>
               <input class="button-primary" type="submit" value="Create">
             </div>
           </div>
          </form>
          {{ with .Numbererr}}
          <div class="row">
            <p class="error">
              {{ range .}}{{.}}{{end}}
            </p>
          </div>
          {{ end }}

       </div>
 </div>
</div>
</body>
</html>
`

// Page to be used in user interface web page
type Page struct {
	Singleerr []string
	Bulkerr   []string
	Numbererr []string
	Year      int
	Body      []byte
	Labelfile string
}

// Output structure for PDF
type Output struct {
	Year     int
	Name     string
	SortName string
	Boat     string
	Berth    string
}

// SurnameSorter for sort package
type SurnameSorter []Output

func (a SurnameSorter) Len() int           { return len(a) }
func (a SurnameSorter) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a SurnameSorter) Less(i, j int) bool { return a[i].SortName < a[j].SortName }

// A map of error strings related to a single print
var singleError = make([]string, 0)

// A map of error strings related to a single print
var numberError = make([]string, 0)

// A map of error strings related to a bulk print
var bulkError = make([]string, 0)

// output PDF file including path
var outputFile string

// allow  serving of local static file
var chttp = http.NewServeMux()


// main is the  handler listens on port 9080 sub nodes process and get redirected back to root
func main() {
	log.Println(">>>>starting up ....")
	port := os.Getenv("PORT")

	if port == "" {
		port = "9080"
		err := open.Run("http://localhost:" + port)
		check(err)
	}

	chttp.Handle("/", http.FileServer(http.Dir("./")))

	http.HandleFunc("/", handler)
	http.HandleFunc("/printsingle", printsingle)
	http.HandleFunc("/printbulk", printbulk)
	http.HandleFunc("/printnumber", printnumber)

	log.Println(">>>>A browser should launch now")

	log.Println(">>>>If you can't see a form, hit refresh in the browser ..")
	log.Println(">>>>Only close this when you are done<<<<")

	log.Fatal(http.ListenAndServe(":"+port, nil))

}

// handler is the root page handler - this is the main user interface
func handler(w http.ResponseWriter, r *http.Request) {
	if strings.Contains(r.URL.Path, ".") {
		// static type file with a dot
		chttp.ServeHTTP(w, r)
	} else {

		// regular home page
		t := time.Now()
		p := &Page{Singleerr: singleError, Bulkerr: bulkError, Numbererr: numberError, Year: t.Year(), Labelfile: outputFile}
		tpl, err := template.New("testp").Parse(testp)
		check(err)
		err = tpl.Execute(w, p)
		check(err)

		singleError = nil
		bulkError = nil
		numberError = nil
	}
}

// form response for /printnumber that processes input
func printnumber(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		fmt.Fprint(w, err)
	}

	from, err := strconv.Atoi(r.Form["From"][0])
	
	if err != nil {
		numberError = append(numberError, "Enter a valid number in FROM")
	}
	to, err := strconv.Atoi(r.Form["To"][0])
	
	if err != nil {
		numberError = append(numberError, " Enter a valid number in TO")
	}
	
	if from > to {
		numberError = append(numberError, "TO cant be less than FROM")
	}


	// redirects back to root if an error
	if len(numberError) > 0 {
		http.Redirect(w, r, "/#number", http.StatusFound)
	} else {
		// build the pdf output 
		
		pdf := gofpdf.New("P", "mm", "A4", "")
		for i := from ; i <= to; i++ {
		     pdfNumberContent(pdf, i)
		}
		outputFile = "numberlabel.pdf"
		pdfOutput(pdf, outputFile)
		http.Redirect(w, r, "/", http.StatusFound)
	}
}

// form response for /printsingle that processes input
func printsingle(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		fmt.Fprint(w, err)
	}
	if len(r.Form["Name"][0]) < 3 {
		singleError = append(singleError, "Error Name too short: ")
	}
	if len(r.Form["Boat"][0]) < 3 {
		singleError = append(singleError, "Error Boat too short: ")
	}
	if len(r.Form["Berth"][0]) == 0 {
		singleError = append(singleError, "Error Berth can not be empty: ")
	}
	// redirects back to root if an error
	if len(singleError) > 0 {
		http.Redirect(w, r, "/", http.StatusFound)
	} else {
		// build the pdf output - note done twice one for boat one for trailer
		t := time.Now()
		pdf := gofpdf.New("P", "mm", "A4", "")
		pdfContent(pdf, t.Year(), r.Form["Name"][0], r.Form["Boat"][0], r.Form["Berth"][0])
		pdfContent(pdf, t.Year(), r.Form["Name"][0], r.Form["Boat"][0], r.Form["Berth"][0])
		outputFile = "singlelabel.pdf"
		pdfOutput(pdf, outputFile)
		http.Redirect(w, r, "/", http.StatusFound)
	}
}

// form response for /bulk that processes input from two csv files, combines them an loops through
// double lable production
func printbulk(w http.ResponseWriter, r *http.Request) {

	file1, _, err := r.FormFile("BerthFile")
	if err != nil {
		bulkError = append(bulkError, fmt.Sprintf("Mooring File: %v", err))
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	berthfile := csv.NewReader(file1)
	berths, err := berthfile.ReadAll()
	if err != nil {
		bulkError = append(bulkError, fmt.Sprintf("Berth File: Error reading all lines: %v", err))
	}

	file2, _, err := r.FormFile("BoatFile")
	if err != nil {
		bulkError = append(singleError, fmt.Sprintf("Boat File: %v", err))
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	boatsfile := csv.NewReader(file2)
	boats, err := boatsfile.ReadAll()
	if err != nil {
		bulkError = append(singleError, fmt.Sprintf("Boats File: Error reading all lines: %v", err))
	}
	if len(bulkError) > 0 {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	t := time.Now()
	var boat string
	var name string
	var nameparts []string
	var oneRecord Output
	var allRecords []Output
	log.Println(">>>>If you can't see a form, hit refresh in the browser ..")
	// loop through the berths
	for _, berth := range berths[1:] {
		if len(berth[16]) < 1{
			continue
		}
		sd, err := time.Parse("02/Jan/2006", berth[16])
		check(err)
		ed, err := time.Parse("02/Jan/2006", berth[17])
		check(err)
		if t.Before(ed) && t.After(sd) {
			boat, name = searchBoats(boats, berth[15])
			oneRecord.Year = t.Year()
			oneRecord.Name = name
			nameparts = strings.Split(name, " ")
			oneRecord.SortName = fmt.Sprintf("%s %s", nameparts[len(nameparts)-1], nameparts[0])
			oneRecord.Boat = boat
			oneRecord.Berth = fmt.Sprintf("%s%s", berth[3], berth[1])
			allRecords = append(allRecords, oneRecord)
		}
	}

	// sort comes here
	sort.Sort(SurnameSorter(allRecords))

	// PDF output
	pdf := gofpdf.New("P", "mm", "A4", "")
	for _, label := range allRecords {
		pdfContent(pdf, label.Year, label.Name, label.Boat, label.Berth)
		pdfContent(pdf, label.Year, label.Name, label.Boat, label.Berth)
	}

	outputFile = "bulklabels.pdf"
	pdfOutput(pdf, outputFile)
	http.Redirect(w, r, "/", http.StatusFound)
}

// searches the boats CSV matching on boat ID
// ignores first line
func searchBoats(records [][]string, boatid string) (string, string) {
	var boat string
	var name string
	for i, line := range records {
		if i > 0 {
			if line[0] == boatid {
				boat = line[1]
				if len(line[44]) > 0 {
					name = line[44]
				} else {
					name = line[33]
				}
				break
			}
		}
	}
	return boat, name
}

// writes the pdf file
func pdfOutput(pdf *gofpdf.Fpdf, o string) {
	err := pdf.OutputFileAndClose(o)
	check(err)
}

// build the pdf content to specific layout and size
func pdfContent(pdf *gofpdf.Fpdf, year int, name string, boat string, berth string) {
	pdf.AddPageFormat("P", gofpdf.SizeType{Wd: 63, Ht: 43})
	pdf.SetMargins(5, 3, 3)
	pdf.SetAutoPageBreak(false, 1)
	pdf.SetFont("Arial", "B", 18)
	pdf.SetXY(5, 3)
	pdf.SetFillColor(0, 0, 0)
	pdf.SetTextColor(255, 255, 255)
	out := fmt.Sprintf("PSC Licence %4d", year)
	pdf.CellFormat(0, 12, out, "1", 0, "C", true, 0, "")
	pdf.Ln(12)
	pdf.SetTextColor(0, 0, 0)
	pdf.SetFont("Arial", "B", 15)
	out = fmt.Sprintf("%s\n%s\n%s", short(name, 16), boat, berth)
	pdf.MultiCell(0, 8, out, "1", "L", false)

}

// build the pdf content to specific layout and size
func pdfNumberContent(pdf *gofpdf.Fpdf, number int) {
	pdf.AddPageFormat("P", gofpdf.SizeType{Wd: 63, Ht: 43})
	pdf.SetMargins(5, 3, 3)
	pdf.SetAutoPageBreak(false, 1)
	pdf.SetXY(5, 3)
	pdf.SetTextColor(0, 0, 0)
	pdf.SetFont("Arial", "B", 58)
	out := fmt.Sprintf("%d", number)
	pdf.CellFormat(0, 37, out, "1", 0, "C", false, 0, "")

}

// short truncates
func short(s string, i int) string {
	runes := []rune(s)
	if len(runes) > i {
		return string(runes[:i])
	}
	return s
}

// check error Fatal
func check(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
