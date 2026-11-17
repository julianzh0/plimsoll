// shim.c: the query half of the C side of plimsoll's parser, compiled once per grammar into one WebAssembly module
// with tree-sitter's core and the grammar's parser. LANGUAGE names the grammar's function (for example
// tree_sitter_go). The host compiles a query once, then runs it per file and reads every capture of every match.
#include <stdlib.h>
#include <string.h>
#include <tree_sitter/api.h>

#ifdef __wasm__
#define EXPORT(name) __attribute__((export_name(name)))
#else
#define EXPORT(name)
#endif

const TSLanguage *LANGUAGE(void);

static TSParser *parser;

EXPORT("shim_alloc") void *shim_alloc(uint32_t size) { return malloc(size); }
EXPORT("shim_free") void shim_free(void *p) { free(p); }

typedef struct { uint32_t match, pattern, capture, start_byte, end_byte, start_row, end_row; } Capture;
static Capture *captures;
static uint32_t capture_count, capture_capacity;
static TSQueryCursor *query_cursor;
static uint32_t query_error_offset;

EXPORT("shim_query_new") TSQuery *shim_query_new(const char *src, uint32_t len) {
  TSQueryError type;
  return ts_query_new(LANGUAGE(), src, len, &query_error_offset, &type);
}
EXPORT("shim_query_error_offset") uint32_t shim_query_error_offset(void) { return query_error_offset; }
EXPORT("shim_query_capture_count") uint32_t shim_query_capture_count(TSQuery *q) { return ts_query_capture_count(q); }
EXPORT("shim_query_capture_name") const char *shim_query_capture_name(TSQuery *q, uint32_t id, uint32_t *len) {
  return ts_query_capture_name_for_id(q, id, len);
}
EXPORT("shim_query_pattern_count") uint32_t shim_query_pattern_count(TSQuery *q) { return ts_query_pattern_count(q); }
// shim_query_predicates writes a pattern's predicate steps as (type, value id) pairs and returns how many steps.
EXPORT("shim_query_predicates") const TSQueryPredicateStep *shim_query_predicates(TSQuery *q, uint32_t pattern, uint32_t *count) {
  return ts_query_predicates_for_pattern(q, pattern, count);
}
EXPORT("shim_query_string") const char *shim_query_string(TSQuery *q, uint32_t id, uint32_t *len) {
  return ts_query_string_value_for_id(q, id, len);
}

static void add_capture(Capture c) {
  if (capture_count == capture_capacity) {
    capture_capacity = capture_capacity ? capture_capacity * 2 : 256;
    Capture *grown = realloc(captures, capture_capacity * sizeof(Capture));
    // A failed allocation must trap, not write through a null pointer: address 0 is valid memory in WebAssembly.
    if (!grown) abort();
    captures = grown;
  }
  captures[capture_count++] = c;
}

EXPORT("shim_query_run") uint32_t shim_query_run(TSQuery *q, const char *src, uint32_t len) {
  if (!parser) {
    parser = ts_parser_new();
    ts_parser_set_language(parser, LANGUAGE());
  }
  if (!query_cursor) query_cursor = ts_query_cursor_new();
  capture_count = 0;
  TSTree *tree = ts_parser_parse_string(parser, NULL, src, len);
  TSNode root = ts_tree_root_node(tree);
  ts_query_cursor_exec(query_cursor, q, root);
  TSQueryMatch m;
  while (ts_query_cursor_next_match(query_cursor, &m)) {
    for (uint16_t i = 0; i < m.capture_count; i++) {
      TSNode n = m.captures[i].node;
      add_capture((Capture){m.id, m.pattern_index, m.captures[i].index, ts_node_start_byte(n), ts_node_end_byte(n),
                            ts_node_start_point(n).row, ts_node_end_point(n).row});
    }
  }
  // The top-level named children follow as matches of pattern 0xFFFFFFFF, for rules about a whole file.
  uint32_t children = ts_node_named_child_count(root);
  for (uint32_t i = 0; i < children; i++) {
    TSNode n = ts_node_named_child(root, i);
    add_capture((Capture){0xFFFFFFFF, 0xFFFFFFFF, 0, ts_node_start_byte(n), ts_node_end_byte(n),
                          ts_node_start_point(n).row, ts_node_end_point(n).row});
  }
  ts_tree_delete(tree);
  return capture_count;
}
EXPORT("shim_query_captures") Capture *shim_query_captures(void) { return captures; }

// The walk: every node of the tree in text order, a parent before its children, into one array sized for the tree.
// flags: 1 named, 2 an ERROR or a MISSING node, 4 the name field of its parent. The host copies the array and releases
// it with shim_walk_release, so a walk keeps no memory between files.
typedef struct { uint16_t kind, flags; uint32_t depth, start_byte, end_byte; } NodeRecord;
static NodeRecord *nodes;

EXPORT("shim_walk") uint32_t shim_walk(const char *src, uint32_t len) {
  if (!parser) {
    parser = ts_parser_new();
    ts_parser_set_language(parser, LANGUAGE());
  }
  TSTree *tree = ts_parser_parse_string(parser, NULL, src, len);
  TSNode root = ts_tree_root_node(tree);
  nodes = malloc((size_t)ts_node_descendant_count(root) * sizeof(NodeRecord));
  if (!nodes) abort(); // address 0 is valid memory in WebAssembly
  TSFieldId name_field = ts_language_field_id_for_name(LANGUAGE(), "name", 4);
  uint32_t count = 0, depth = 0;
  TSTreeCursor c = ts_tree_cursor_new(root);
  for (;;) {
    TSNode n = ts_tree_cursor_current_node(&c);
    uint16_t flags = (ts_node_is_named(n) ? 1 : 0) | (ts_node_is_error(n) || ts_node_is_missing(n) ? 2 : 0) |
                     (name_field && ts_tree_cursor_current_field_id(&c) == name_field ? 4 : 0);
    nodes[count++] = (NodeRecord){ts_node_symbol(n), flags, depth, ts_node_start_byte(n), ts_node_end_byte(n)};
    if (ts_tree_cursor_goto_first_child(&c)) { depth++; continue; }
    while (!ts_tree_cursor_goto_next_sibling(&c)) {
      if (!ts_tree_cursor_goto_parent(&c)) goto done;
      depth--;
    }
  }
done:
  ts_tree_cursor_delete(&c);
  ts_tree_delete(tree);
  return count;
}
EXPORT("shim_nodes") NodeRecord *shim_nodes(void) { return nodes; }
EXPORT("shim_walk_release") void shim_walk_release(void) {
  free(nodes);
  nodes = NULL;
}
EXPORT("shim_kind_count") uint32_t shim_kind_count(void) { return ts_language_symbol_count(LANGUAGE()); }
EXPORT("shim_kind_name") const char *shim_kind_name(uint32_t id, uint32_t *len) {
  const char *name = ts_language_symbol_name(LANGUAGE(), id);
  *len = strlen(name);
  return name;
}
