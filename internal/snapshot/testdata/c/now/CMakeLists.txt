cmake_minimum_required(VERSION 3.20)
project(tally C)

add_library(tally src/tally.c src/buffer.c)
target_include_directories(tally PUBLIC include)

add_executable(test_tally tests/test_tally.c)
target_link_libraries(test_tally PRIVATE tally)
