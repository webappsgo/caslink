# Translating Caslink

Caslink uses `src/common/i18n/locales/en.json` as the source for user-facing
translation keys. The server and client binaries embed the same locale files.

## Supported languages

The repository currently ships eight locale files:

| Code | Language | Direction |
|------|----------|-----------|
| `ar` | Arabic | Right to left |
| `de` | German | Left to right |
| `en` | English | Left to right |
| `es` | Spanish | Left to right |
| `fr` | French | Left to right |
| `it` | Italian | Left to right |
| `ja` | Japanese | Left to right |
| `zh` | Chinese | Left to right |

## Update a translation

1. Edit only the locale file for the language being updated.
2. Keep the English file's JSON keys, nesting, and key order. Translate values,
   not keys.
3. Preserve named placeholders exactly, including their braces, such as
   `{count}` and `{datetime}`.
4. Keep all required plural categories for that locale.
5. Use complete, natural sentences and consistent formal address. Follow the
   language's punctuation and typography conventions.
6. Check the result at 320, 768, and 1280 pixels, including right-to-left
   layout for Arabic. Long translations must wrap without clipping or overflow.
7. Run `make test` in the supported Go container. The locale checks compare
   the translation keys and placeholders with English.

## Add a language

Use a lowercase BCP 47 code for the filename, such as `pt.json` or
`pt-br.json`. Copy the English locale structure, translate every value, and
fill in the locale metadata: language code, English name, native name,
direction, and version. Add the language to the shared language registry and
the default available-language configuration. Update the supported-language
list in the README and this page in the same change. Rebuild the server and
client so both embed the new locale.

## Translation conventions

- Keep product names, protocol names, code, file names, configuration keys,
  URLs, and literal values such as `dark` and `auto` unchanged.
- Treat placeholders as data: preserve their exact spelling and braces.
- Translate each complete message as a whole; do not concatenate sentence
  fragments across keys.
- Use consistent terminology for authentication tokens, links, organizations,
  sessions, and security actions throughout a locale.
- Check text expansion and fonts for scripts that may not be covered by the
  system font stack.
- Report unclear English source text so it can be clarified in the source
  locale rather than worked around in translations.

## Review and credits

Reviewers check key parity, placeholders, plural categories, terminology, and
the layouts at the widths above. Native-speaker review is welcome but is not
a merge requirement. Translators may choose a name or handle for attribution;
add it to `meta.translators` in the locale file and this page. No translator
credits are currently recorded in the locale metadata.
