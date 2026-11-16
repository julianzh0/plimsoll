find_package(GTest REQUIRED)

add_executable(store_test store_test.cc)
target_include_directories(store_test PRIVATE ${CMAKE_CURRENT_SOURCE_DIR}/support)
target_link_libraries(store_test PRIVATE notes GTest::gtest_main)
