# Cookies Service

## Collection

Cookies are collected from every response independent of cookie-mode using `curl --cookie-yar <filename>`
and write step.response.cookie and save to CookiesKeyValueStore using method SetAll(<read jar filename>).

## Mode and Propagation

- cookie key [name]:[domain]:[path]
- in cookie_mode: included - include cookie_keys - use ckvs.GetIncluded([]string) string - returns <cookie-values>
- in cookie_mode: excluded - exclude cookie_keys - use ckvs.GetExcluded([]string) string - returns <cookie-values>
- propagate values from root to steps
- step has cookies member representing concated cookies values extracted by one of the above commands
- cookies are passed to request using `curl --cookie "<cookie-values>"` if request.cookies
