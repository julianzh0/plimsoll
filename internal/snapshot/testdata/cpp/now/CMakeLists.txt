cmake_minimum_required(VERSION 3.20)
project(notes CXX)

find_package(fmt REQUIRED)

add_library(notes src/store.cc)
target_include_directories(notes
  PUBLIC $<BUILD_INTERFACE:${PROJECT_SOURCE_DIR}/include> $<INSTALL_INTERFACE:include>
  PRIVATE src)
target_link_libraries(notes PRIVATE fmt::fmt)

add_executable(notes-cli src/main.cpp)
target_link_libraries(notes-cli PRIVATE notes)

add_subdirectory(tests)
